package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: gen-api-doc.go <output-file>\n")
		os.Exit(1)
	}

	outputPath := os.Args[1]
	repoRoot := findRepoRoot()

	var sb strings.Builder
	sb.WriteString("# API Reference\n\n")
	sb.WriteString("> Auto-generated from source. Do not edit manually.\n\n")

	for _, pc := range []struct {
		name       string
		dir        string
		importPath string
	}{
		{"taskrunner", ".", "taskrunner"},
		{"core", "core", "core"},
	} {
		pkgDir := filepath.Join(repoRoot, pc.dir)
		pkgDoc := parsePackage(pkgDir, pc.importPath)
		writePackage(&sb, pkgDoc)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", outputPath)
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintf(os.Stderr, "could not find go.mod\n")
			os.Exit(1)
		}
		dir = parent
	}
}

type pkgDoc struct {
	Name         string
	ImportPath   string
	Doc          string
	Interfaces   []typeEntry
	Structs      []typeEntry
	Types        []typeEntry
	Constructors []funcEntry
	Functions    []funcEntry
	Constants    []valueEntry
	Variables    []valueEntry
}

type typeEntry struct {
	Name    string
	Doc     string
	Decl    string
	Methods []methodEntry
}

type methodEntry struct {
	Name string
	Doc  string
	Decl string
}

type funcEntry struct {
	Name string
	Doc  string
	Decl string
}

type valueEntry struct {
	Names []string
	Doc   string
	Decl  string
}

func parsePackage(dir, importPath string) *pkgDoc {
	fset := token.NewFileSet()
	filter := func(fi os.FileInfo) bool {
		name := fi.Name()
		return !fi.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}
	pkgs, err := parser.ParseDir(fset, dir, filter, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", dir, err)
		os.Exit(1)
	}

	var astPkg *ast.Package
	var pkgName string
	for name, p := range pkgs {
		astPkg = p
		pkgName = name
		break
	}

	d := doc.New(astPkg, importPath, doc.AllDecls)

	result := &pkgDoc{
		Name:       pkgName,
		ImportPath: importPath,
		Doc:        strings.TrimSpace(d.Doc),
	}

	for _, t := range d.Types {
		if !ast.IsExported(t.Name) {
			continue
		}

		entry := typeEntry{
			Name: t.Name,
			Doc:  strings.TrimSpace(t.Doc),
		}

		if t.Decl != nil {
			entry.Decl = strings.TrimSpace(nodeText(fset, t.Decl))
		}

		isIface := isInterfaceDecl(t.Decl)

		var exportedMethods []methodEntry
		for _, m := range t.Methods {
			if !ast.IsExported(m.Name) {
				continue
			}
			sig := ""
			if m.Decl != nil && m.Decl.Type != nil {
				sig = strings.TrimSpace(nodeText(fset, m.Decl.Type))
			}
			exportedMethods = append(exportedMethods, methodEntry{
				Name: m.Name,
				Doc:  strings.TrimSpace(m.Doc),
				Decl: sig,
			})
		}
		sort.Slice(exportedMethods, func(i, j int) bool {
			return exportedMethods[i].Name < exportedMethods[j].Name
		})
		entry.Methods = exportedMethods

		if isIface {
			result.Interfaces = append(result.Interfaces, entry)
		} else if isStructDecl(t.Decl) {
			result.Structs = append(result.Structs, entry)
		} else {
			result.Types = append(result.Types, entry)
		}
	}

	sortByName(result.Interfaces)
	sortByName(result.Structs)
	sortByName(result.Types)

	for _, f := range d.Funcs {
		if !ast.IsExported(f.Name) {
			continue
		}
		decl := formatFuncDecl(fset, f.Decl)
		if strings.HasPrefix(f.Name, "New") || strings.HasPrefix(f.Name, "Default") {
			result.Constructors = append(result.Constructors, funcEntry{
				Name: f.Name,
				Doc:  strings.TrimSpace(f.Doc),
				Decl: decl,
			})
		} else {
			result.Functions = append(result.Functions, funcEntry{
				Name: f.Name,
				Doc:  strings.TrimSpace(f.Doc),
				Decl: decl,
			})
		}
	}
	for _, t := range d.Types {
		for _, f := range t.Funcs {
			if !ast.IsExported(f.Name) {
				continue
			}
			decl := formatFuncDecl(fset, f.Decl)
			if strings.HasPrefix(f.Name, "New") || strings.HasPrefix(f.Name, "Default") || strings.HasPrefix(f.Name, "Create") {
				result.Constructors = append(result.Constructors, funcEntry{
					Name: f.Name,
					Doc:  strings.TrimSpace(f.Doc),
					Decl: decl,
				})
			} else {
				result.Functions = append(result.Functions, funcEntry{
					Name: f.Name,
					Doc:  strings.TrimSpace(f.Doc),
					Decl: decl,
				})
			}
		}
	}

	sort.Slice(result.Constructors, func(i, j int) bool {
		return result.Constructors[i].Name < result.Constructors[j].Name
	})
	sort.Slice(result.Functions, func(i, j int) bool {
		return result.Functions[i].Name < result.Functions[j].Name
	})

	for _, t := range d.Types {
		for _, c := range t.Consts {
			exported := make([]string, 0, len(c.Names))
			for _, n := range c.Names {
				if ast.IsExported(n) {
					exported = append(exported, n)
				}
			}
			if len(exported) == 0 {
				continue
			}
			decl := ""
			if c.Decl != nil {
				decl = strings.TrimSpace(nodeText(fset, c.Decl))
			}
			result.Constants = append(result.Constants, valueEntry{
				Names: exported,
				Doc:   strings.TrimSpace(c.Doc),
				Decl:  decl,
			})
		}
	}
	for _, c := range d.Consts {
		exported := make([]string, 0, len(c.Names))
		for _, n := range c.Names {
			if ast.IsExported(n) {
				exported = append(exported, n)
			}
		}
		if len(exported) == 0 {
			continue
		}
		decl := ""
		if c.Decl != nil {
			decl = strings.TrimSpace(nodeText(fset, c.Decl))
		}
		result.Constants = append(result.Constants, valueEntry{
			Names: exported,
			Doc:   strings.TrimSpace(c.Doc),
			Decl:  decl,
		})
	}
	sort.Slice(result.Constants, func(i, j int) bool {
		return result.Constants[i].Names[0] < result.Constants[j].Names[0]
	})

	for _, v := range d.Vars {
		exported := make([]string, 0, len(v.Names))
		for _, n := range v.Names {
			if ast.IsExported(n) {
				exported = append(exported, n)
			}
		}
		if len(exported) == 0 {
			continue
		}
		decl := ""
		if v.Decl != nil {
			decl = strings.TrimSpace(nodeText(fset, v.Decl))
		}
		result.Variables = append(result.Variables, valueEntry{
			Names: exported,
			Doc:   strings.TrimSpace(v.Doc),
			Decl:  decl,
		})
	}
	sort.Slice(result.Variables, func(i, j int) bool {
		return result.Variables[i].Names[0] < result.Variables[j].Names[0]
	})

	return result
}

func isInterfaceDecl(decl *ast.GenDecl) bool {
	if decl == nil {
		return false
	}
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if _, ok := ts.Type.(*ast.InterfaceType); ok {
			return true
		}
	}
	return false
}

func isStructDecl(decl *ast.GenDecl) bool {
	if decl == nil {
		return false
	}
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if _, ok := ts.Type.(*ast.StructType); ok {
			return true
		}
	}
	return false
}

func nodeText(fset *token.FileSet, node ast.Node) string {
	pos := fset.Position(node.Pos())
	end := fset.Position(node.End())
	data, err := os.ReadFile(pos.Filename)
	if err != nil {
		return ""
	}
	startOff := pos.Offset
	endOff := end.Offset
	if startOff < 0 || endOff > len(data) || startOff >= endOff {
		return ""
	}
	return string(data[startOff:endOff])
}

func formatFuncDecl(fset *token.FileSet, decl *ast.FuncDecl) string {
	if decl == nil {
		return ""
	}
	return strings.TrimSpace(nodeText(fset, decl))
}

func sortByName(entries []typeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
}

func writePackage(sb *strings.Builder, pd *pkgDoc) {
	sb.WriteString(fmt.Sprintf("## Package `%s`\n\n", pd.Name))
	if pd.Doc != "" {
		sb.WriteString(pd.Doc + "\n\n")
	}

	type section struct {
		title  string
		hasAny bool
		write  func()
	}

	sections := []section{
		{"Interfaces", len(pd.Interfaces) > 0, func() {
			for _, t := range pd.Interfaces {
				writeTypeEntry(sb, t, true)
			}
		}},
		{"Structs", len(pd.Structs) > 0, func() {
			for _, t := range pd.Structs {
				writeTypeEntry(sb, t, false)
			}
		}},
		{"Types", len(pd.Types) > 0, func() {
			for _, t := range pd.Types {
				writeTypeEntry(sb, t, false)
			}
		}},
		{"Constructors", len(pd.Constructors) > 0, func() {
			for _, f := range pd.Constructors {
				writeFuncEntry(sb, f)
			}
		}},
		{"Functions", len(pd.Functions) > 0, func() {
			for _, f := range pd.Functions {
				writeFuncEntry(sb, f)
			}
		}},
		{"Constants", len(pd.Constants) > 0, func() {
			for _, c := range pd.Constants {
				writeValueEntry(sb, c)
			}
		}},
		{"Variables", len(pd.Variables) > 0, func() {
			for _, v := range pd.Variables {
				writeValueEntry(sb, v)
			}
		}},
	}

	for _, s := range sections {
		if !s.hasAny {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n", s.title))
		s.write()
	}
}

func writeTypeEntry(sb *strings.Builder, t typeEntry, isInterface bool) {
	sb.WriteString(fmt.Sprintf("#### `%s`\n\n", t.Name))
	if t.Doc != "" {
		sb.WriteString(t.Doc + "\n\n")
	}
	if t.Decl != "" {
		sb.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", t.Decl))
	}
	if len(t.Methods) > 0 {
		if isInterface {
			sb.WriteString("**Methods:**\n\n")
		} else {
			sb.WriteString("**Exported methods:**\n\n")
		}
		for _, m := range t.Methods {
			sb.WriteString(fmt.Sprintf("- `%s`", m.Name))
			if m.Decl != "" {
				sb.WriteString(fmt.Sprintf(" %s", m.Decl))
			}
			sb.WriteString("\n")
			if m.Doc != "" {
				for _, line := range strings.Split(m.Doc, "\n") {
					sb.WriteString("  > " + line + "\n")
				}
				sb.WriteString("\n")
			}
		}
	}
}

func writeFuncEntry(sb *strings.Builder, f funcEntry) {
	sb.WriteString(fmt.Sprintf("#### `%s`\n\n", f.Name))
	if f.Doc != "" {
		sb.WriteString(f.Doc + "\n\n")
	}
	if f.Decl != "" {
		sb.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", f.Decl))
	}
}

func writeValueEntry(sb *strings.Builder, v valueEntry) {
	joined := "`" + strings.Join(v.Names, "`, `") + "`"
	sb.WriteString(fmt.Sprintf("- %s", joined))
	if v.Decl != "" {
		lines := strings.Split(strings.TrimSpace(v.Decl), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				val := strings.TrimSpace(parts[1])
				if val != "" {
					sb.WriteString(fmt.Sprintf(" = %s", val))
				}
			}
		}
	}
	sb.WriteString("\n")
	if v.Doc != "" {
		for _, line := range strings.Split(v.Doc, "\n") {
			sb.WriteString("  > " + line + "\n")
		}
		sb.WriteString("\n")
	}
}
