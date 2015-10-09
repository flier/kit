package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strings"
	"text/template"

	"go/ast"
	"go/format"
	"go/token"
)

var (
	topLevelTemplate = template.New("/").Funcs(template.FuncMap{
		"join":         func(a []string) string { return strings.Join(a, ",") },
		"capitalize":   func(s string) string { return strings.ToTitle(s) },
		"uncapitalize": func(s string) string { return strings.ToLower(s[:1]) + s[1:] },
		"upper":        func(s string) string { return strings.ToUpper(s) },
		"lower":        func(s string) string { return strings.ToLower(s) },
	})
)

func loadTemplate(name string) (*template.Template, error) {
	log.Printf("loading template: %s", name)

	for _, tmpl := range topLevelTemplate.Templates() {
		if tmpl.Name() == name {
			return tmpl, nil
		}
	}

	if f, err := FS(*debugMode).Open(name); err != nil {
		return nil, fmt.Errorf("fail to open template, %s", err)
	} else if data, err := ioutil.ReadAll(f); err != nil {
		return nil, fmt.Errorf("fail to read template, %s", err)
	} else if tmpl, err := topLevelTemplate.Parse(string(data)); err != nil {
		return nil, fmt.Errorf("fail to parse template, %s", err)
	} else {
		return tmpl, nil
	}
}

type Render struct {
	buf bytes.Buffer // Accumulated output.
}

func (r *Render) Append(text string) *Render {
	r.buf.WriteString(text)

	return r
}

func (r *Render) Printf(format string, args ...interface{}) {
	fmt.Fprintf(&r.buf, format, args...)
}

// format returns the gofmt-ed contents of the Generator's buffer.
func (r *Render) format() ([]byte, error) {
	if src, err := format.Source(r.buf.Bytes()); err != nil {
		return r.buf.Bytes(), err
	} else {
		return src, nil
	}
}

type TypeRender struct {
	name  string
	buf   bytes.Buffer
	stack []token.Pos
}

func (r *TypeRender) render() string {
	return r.buf.String()
}

// genDecl processes one declaration clause.
func (r *TypeRender) visit(node ast.Node) bool {
	if node == nil {
		return true
	}

	if *debugMode {
		buf := new(bytes.Buffer)

		for len(r.stack) > 0 && r.stack[len(r.stack)-1] < node.Pos() {
			r.stack = r.stack[:len(r.stack)-1]
		}

		r.stack = append(r.stack, node.End())

		fmt.Fprintf(buf, "%p", node)

		for i := 0; i <= len(r.stack)-1; i++ {
			buf.WriteString("  ")
		}

		fmt.Fprintf(buf, "%s [%d:%d] %v", reflect.TypeOf(node), node.Pos(), node.End(), node)

		log.Print(buf.String())
	}

	if decl, ok := node.(*ast.GenDecl); ok && decl.Tok == token.TYPE {
		if decorators := r.parseDecorators(decl.Doc); len(decorators) > 0 {
			if *debugMode {
				log.Printf("found decorators: %v", decorators)
			}

			if specs := r.parseSpecs(decl.Specs); len(specs) > 0 {
				if *debugMode {
					if b, err := json.Marshal(specs); err != nil {
						log.Fatal(err)
					} else {
						var out bytes.Buffer

						json.Indent(&out, b, ">", "\t")

						log.Printf("found specs: %s", out.String())
					}
				}

				for decorator, names := range decorators {
					for _, name := range names {
						if tmpl, err := loadTemplate(fmt.Sprintf("/templates/%s/%s.tmpl", decorator, name)); err != nil {
							log.Fatal(err)
						} else if err := tmpl.Execute(&r.buf, specs); err != nil {
							log.Fatal(err)
						}
					}
				}
			}
		}
	}

	return true
}

func (r *TypeRender) parseDecorators(comments *ast.CommentGroup) (decorators map[string][]string) {
	if comments != nil {
		for _, comment := range comments.List {
			if strings.HasPrefix(comment.Text, kitCommentPrefix) {
				line := comment.Text[len(kitCommentPrefix):]

				parts := kitCommentSep.Split(line, 2)

				name := strings.TrimSpace(parts[0])
				var params []string

				if len(parts) > 1 {
					r := csv.NewReader(strings.NewReader(parts[1]))
					r.Comment = '#'
					r.TrimLeadingSpace = true

					if fields, err := r.Read(); err == nil {
						params = fields
					}
				}

				if decorators == nil {
					decorators = map[string][]string{name: params}
				} else if _, exists := decorators[name]; exists && params != nil {
					decorators[name] = append(decorators[name], params...)
				} else {
					decorators[name] = params
				}
			}
		}
	}

	return
}

func (r *TypeRender) parseSpecs(specs []ast.Spec) (types []map[string]interface{}) {
	for _, spec := range specs {
		if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name != nil && typeSpec.Name.Name == r.name {
			if intfType, ok := typeSpec.Type.(*ast.InterfaceType); ok {
				var methods []interface{}

				for _, method := range intfType.Methods.List {
					if funcType, ok := method.Type.(*ast.FuncType); ok {
						methods = append(methods, map[string]interface{}{
							"Name":    method.Names[0].Name,
							"Params":  r.parseFieldList(funcType.Params),
							"Results": r.parseFieldList(funcType.Results),
						})
					}
				}

				types = append(types, map[string]interface{}{
					"Name":    typeSpec.Name.Name,
					"Methods": methods,
				})
			}
		}
	}

	return
}

func (r *TypeRender) parseFieldList(fields *ast.FieldList) (types []map[string]interface{}) {
	i := 0

	for _, field := range fields.List {
		var names []string

		typeName := field.Type.(*ast.Ident).Name

		if len(field.Names) > 0 {
			for _, name := range field.Names {
				names = append(names, strings.Title(name.Name))
			}
		} else {
			names = append(names, fmt.Sprintf("%s%d", strings.Title(typeName[:1]), i))
			i += 1
		}

		for _, name := range names {
			types = append(types, map[string]interface{}{
				"Name": name,
				"Type": typeName,
			})
		}
	}

	return
}
