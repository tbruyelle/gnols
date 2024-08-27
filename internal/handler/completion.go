package handler

import (
	"context"
	"go/ast"
	gotoken "go/token"
	"log/slog"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jdkato/gnols/internal/gno"
	"github.com/jdkato/gnols/internal/stdlib"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"golang.org/x/tools/go/ast/astutil"
)

func (h *handler) handleTextDocumentCompletion(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}

	token, err := doc.TokenAt(params.Position)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	text := strings.TrimSpace(token.Text)
	// Extract symbol name and prefix from text
	// TODO ensure textDocument/completion is only triggered after a dot '.' and
	// if yes why ? Is it a client limitation or configuration, or a LSP spec?
	selectors := strings.FieldsFunc(text, func(r rune) bool { return r == '.' })

	slog.Info("completion", "text", text, "selectors", selectors)

	items := []protocol.CompletionItem{}

	pos := gotoken.Pos(doc.PositionToOffset(params.Position))
	nodes, _ := astutil.PathEnclosingInterval(doc.Pgf.File, pos, pos)
	spew.Dump("ENCLOSING NODES", nodes)

	for _, n := range nodes {
		var syms []gno.Symbol
		switch n := n.(type) {
		case *ast.Ident:
			// related test-cases:
			// - TestScripts/document_completion_local_struct_novar

			// TODO check if it works when multiple selectors
			syms = symbolFinder{h.currentPkg.Symbols}.find([]string{n.Name})

		case *ast.BlockStmt:
			// related test-cases:
			// - TestScripts/document_completion_local_struct
			// - TestScripts/document_completion_local_struct_one_selector
			// - TestScripts/document_completion_local_struct_two_selectors
			// - TestScripts/document_completion_local_struct_three_selectors_inline
			// - TestScripts/document_completion_local_struct_three_selectors

			// Check if selectors[0] has been assigned here
			for _, t := range n.List {
				switch t := t.(type) {
				case *ast.AssignStmt:
					if nodeName(t.Lhs[0]) == selectors[0] && t.Tok == gotoken.DEFINE {
						// found selectors[0] assignment, now fetch type
						if cl, ok := t.Rhs[0].(*ast.CompositeLit); ok {
							typ := nodeName(cl.Type)
							// FIXME typ can come from an other package
							syms = symbolFinder{h.currentPkg.Symbols}.find(append([]string{typ}, selectors[1:]...))
							break
						}
					}
				}
			}

		case *ast.SelectorExpr:
			switch x := n.X.(type) {
			case *ast.Ident:
				// related test-cases:
				// - TestScripts/document_completion_subpackage
				// - TestScripts/document_completion_examples_ufmt
				// - TestScripts/document_completion_stdlib_bufio
				// - TestScripts/document_completion_subpackage_multiple_selectors
				// - TestScripts/document_completion_stdlib_strconv

				// look up in subpackages (TODO also add imported packages)
				pkg := x.Name
				// look up pkg in subpkgs
				for _, sub := range h.subPkgs {
					if sub.Name == pkg {
						syms = symbolFinder{sub.Symbols}.find(selectors[1:])
						break
					}
				}
				if len(syms) == 0 {
					// look up in stdlib
					for _, stdPkg := range stdlib.Packages {
						if stdPkg.Name == pkg {
							syms = symbolFinder{stdPkg.Symbols}.find(selectors[1:])
							break
						}
					}
				}

			case *ast.CallExpr:
				// related test-cases:
				// - TestScripts/document_completion_func_ret

				// this a call, find return type
				// TODO try to replace selectors with the func name without the ()
				// so we dont need to remove them in symbolFinder
				// See if it works with multiple selectors.
				syms = symbolFinder{h.currentPkg.Symbols}.find(selectors)
			}

		case *ast.FuncDecl:
			for _, param := range n.Type.Params.List {
				if param.Names[0].Name == selectors[0] {
					// match, find corresponding type
					switch t := param.Type.(type) {
					case *ast.Ident:
						// related test-cases:
						// - TestScripts/document_completion_func_args
						// - TestScripts/document_completion_func_args3c
						// - TestScripts/document_completion_func_args3b
						// - TestScripts/document_completion_func_args6

						typ := t.Name
						syms = symbolFinder{h.currentPkg.Symbols}.find(append([]string{typ}, selectors[1:]...))

					case *ast.SelectorExpr:
						// related test-cases:
						// - TestScripts/document_completion_func_args2
						// - TestScripts/document_completion_func_args4
						// - TestScripts/document_completion_func_args5

						pkg := t.X.(*ast.Ident).Name
						typ := t.Sel.Name
						// look up pkg in subpkgs
						for _, sub := range h.subPkgs {
							if sub.Name == pkg {
								syms = symbolFinder{sub.Symbols}.find(append([]string{typ}, selectors[1:]...))
								break
							}
						}
						if len(syms) == 0 {
							// look up in stdlib
							for _, stdPkg := range stdlib.Packages {
								if stdPkg.Name == pkg {
									syms = symbolFinder{stdPkg.Symbols}.find(append([]string{typ}, selectors[1:]...))
									break
								}
							}
						}

					case *ast.InterfaceType:
						// related test-cases:
						// - TestScripts/document_completion_func_args3

						// TODO address when len(selectors)>1
						for _, method := range t.Methods.List {
							syms = append(syms, gno.Symbol{
								Name:      method.Names[0].Name,
								Kind:      "method",
								Doc:       method.Comment.Text(),
								Signature: doc.Content[method.Pos()-1 : method.End()-1],
							})
						}

					default:
						panic("FIXME cannot find type")
					}
				}
			}

		case *ast.File:
			// final node, check for global variable declaration that could match
			// selectors[0]
			for _, t := range n.Decls {
				if len(syms) > 0 {
					break
				}
				if d, ok := t.(*ast.GenDecl); ok {
					for _, t := range d.Specs {
						if v, ok := t.(*ast.ValueSpec); ok {
							// FIXME loop over all names in case there's multiple assignement
							if v.Names[0].Name == selectors[0] {
								if len(v.Values) > 0 {
									// related test-cases:
									// - TestScripts/document_completion_local_struct_global_var
									switch vs := v.Values[0].(type) {
									case *ast.CompositeLit:
										typ := nodeName(vs.Type)
										syms = symbolFinder{h.currentPkg.Symbols}.find(append([]string{typ}, selectors[1:]...))
									}
								} else {
									// related test-cases:
									// - TestScripts/document_completion_local_inline_struct
									// TODO address when len(selectors)>1
									for _, field := range v.Type.(*ast.StructType).Fields.List {
										syms = append(syms, gno.Symbol{
											Name:      field.Names[0].Name,
											Kind:      "field",
											Doc:       strings.TrimSpace(field.Doc.Text()),
											Signature: doc.Content[field.Pos()-1 : field.End()-1],
										})
									}
								}
							}
						}
					}
				}
			}
		}

		if len(syms) > 0 {
			spew.Dump("FOUND", syms)
			for _, f := range syms {
				items = append(items, protocol.CompletionItem{
					Label:         f.Name,
					InsertText:    f.Name,
					Kind:          symbolToKind(f.Kind),
					Detail:        f.Signature,
					Documentation: f.Doc,
				})
			}
			break
		}
	}

	/*
		//-----------------------------------------
		// Look up local symbols
		if syms := h.lookupSymbols(selectors); len(syms) > 0 {
			for _, f := range syms {
				items = append(items, protocol.CompletionItem{
					Label:         f.Name,
					InsertText:    f.Name,
					Kind:          symbolToKind(f.Kind),
					Detail:        f.Signature,
					Documentation: f.Doc,
				})
			}
		} else {
			//-----------------------------------------
			// Look up stdlib
			if pkg := lookupPkg(stdlib.Packages, selectors[0]); pkg != nil {
				for _, s := range pkg.Symbols {
					if s.Recv != "" {
						// skip symbols with receiver (methods)
						continue
					}
					if len(selectors) > 1 && !strings.HasPrefix(s.Name, selectors[1]) {
						// TODO handle multiple selectors? (possible if for example a global
						// var is defined in the pkg, and the user is referrencing it.)

						// skip symbols that doesn't match the prefix
						continue
					}
					items = append(items, protocol.CompletionItem{
						Label:         s.Name,
						InsertText:    s.Name,
						Kind:          symbolToKind(s.Kind),
						Detail:        s.Signature,
						Documentation: s.Doc,
					})
				}
			}
		}
	*/
	return reply(ctx, items, nil)
}

/* TODO remove me when astutil.PathEnclosingInterval is proven to be the
 best way to perform code completion

func (h handler) lookupSymbols(selectors []string) []gno.Symbol {
	// Check first in current pkg
	found := symbolFinder{h.currentPkg.Symbols}.find(selectors)
	if len(found) > 0 {
		return found
	}
	// Check in sub packages
	for _, pkg := range h.subPkgs {
		if pkg.Name == selectors[0] {
			return symbolFinder{pkg.Symbols}.find(selectors[1:])
		}
	}
	// TODO check in imported packages of the file
	return []gno.Symbol{}
}
*/

type symbolFinder struct {
	baseSymbols []gno.Symbol
}

func (s symbolFinder) find(selectors []string) []gno.Symbol {
	return s.findIn(s.baseSymbols, selectors)
}

func (s symbolFinder) findIn(symbols []gno.Symbol, selectors []string) []gno.Symbol {
	slog.Info("lookup symbol", "symbols", symbols, "selectors", selectors)
	if len(selectors) == 0 {
		return symbols
	}
	name := selectors[0]
	name = strings.TrimSuffix(name, "()") // TODO add case like func_ret with func arguementsj
	var syms []gno.Symbol
	for _, sym := range symbols {
		switch {
		case sym.Name == name: // exact match
			slog.Info("found symbol", "name", name, "kind", sym.Kind, "type", sym.Type, "selectors", selectors)
			// we found a symbol matching name
			switch sym.Kind {
			case "var", "field":
				if sym.Type == "" {
					// sym is an inline struct, returns fields
					// TODO ensure that works when there's still other selectors
					return sym.Fields
				}
				// lookup for symbols matching type in baseSymbols
				return s.findIn(s.baseSymbols, append([]string{sym.Type}, selectors[1:]...))

			case "struct", "interface":
				// sym is a struct or an interface, lookup in fields/methods
				return s.findIn(sym.Fields, selectors[1:])

			case "func", "method":
				if sym.Type != "" {
					return s.findIn(s.baseSymbols, append([]string{sym.Type}, selectors[1:]...))
				}
			}

		case strings.HasPrefix(sym.Name, name): // partial match
			syms = append(syms, sym)
		}
	}
	return syms
}

func nodeName(n ast.Node) string {
	if id, ok := n.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
