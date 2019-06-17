// General Bazel-related warnings

package warn

import (
	"fmt"
	"strings"

	"github.com/bazelbuild/buildtools/build"
)

func constantGlobWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type == build.TypeDefault {
		// Only applicable to Bazel files
		return
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		call, ok := expr.(*build.CallExpr)
		if !ok || len(call.List) == 0 {
			return
		}
		ident, ok := (call.X).(*build.Ident)
		if !ok || ident.Name != "glob" {
			return
		}
		patterns, ok := call.List[0].(*build.ListExpr)
		if !ok {
			return
		}
		for _, expr := range patterns.List {
			str, ok := expr.(*build.StringExpr)
			if !ok {
				continue
			}
			if !strings.Contains(str.Value, "*") {
				message := fmt.Sprintf(
					`Glob pattern %q has no wildcard ('*'). Constant patterns can be error-prone, move the file outside the glob.`, str.Value)
				findings <- makeLinterFinding(expr, message)
				return // at most one warning per glob
			}
		}
	})
}

func nativeInBuildFilesWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type != build.TypeBuild {
		return
	}

	build.WalkPointers(f, func(expr *build.Expr, stack []build.Expr) {
		// Search for `native.xxx` nodes
		dot, ok := (*expr).(*build.DotExpr)
		if !ok {
			return
		}
		ident, ok := dot.X.(*build.Ident)
		if !ok || ident.Name != "native" {
			return
		}

		findings <- makeLinterFinding(ident,
			`The "native" module shouldn't be used in BUILD files, its members are available as global symbols.`,
			LinterReplacement{expr, &build.Ident{Name: dot.Name}})
	})
}

func nativePackageWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type != build.TypeBzl {
		return
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		// Search for `native.package()` nodes
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		dot, ok := call.X.(*build.DotExpr)
		if !ok || dot.Name != "package" {
			return
		}
		ident, ok := dot.X.(*build.Ident)
		if !ok || ident.Name != "native" {
			return
		}

		findings <- makeLinterFinding(call, `"native.package()" shouldn't be used in .bzl files.`)
	})
}

func duplicatedNameWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type == build.TypeBzl || f.Type == build.TypeDefault {
		// Not applicable to .bzl files.
		return
	}

	names := make(map[string]int) // map from name to line number
	msg := `A rule with name %q was already found on line %d. ` +
		`Even if it's valid for Blaze, this may confuse other tools. ` +
		`Please rename it and use different names.`

	for _, rule := range f.Rules("") {
		name := rule.ExplicitName()
		if name == "" {
			continue
		}
		start, _ := rule.Call.Span()
		if line, ok := names[name]; ok {
			finding := makeLinterFinding(rule.Call, fmt.Sprintf(msg, name, line))
			if nameNode := rule.Attr("name"); nameNode != nil {
				finding.Start, finding.End = nameNode.Span()
				start = finding.Start
			}
			findings <- finding
		} else {
			names[name] = start.Line
		}
	}
}

func positionalArgumentsWarning(call *build.CallExpr, findings chan *LinterFinding) {
	msg := "All calls to rules or macros should pass arguments by keyword (arg_name=value) syntax."
	if id, ok := call.X.(*build.Ident); !ok || functionsWithPositionalArguments[id.Name] {
		return
	}
	for _, arg := range call.List {
		if _, ok := arg.(*build.AssignExpr); ok {
			continue
		}
		findings <- makeLinterFinding(arg, msg)
		return
	}
}

func argsKwargsInBuildFilesWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type != build.TypeBuild {
		return
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		// Search for function call nodes
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		for _, param := range call.List {
			unary, ok := param.(*build.UnaryExpr)
			if !ok {
				continue
			}
			switch unary.Op {
			case "*":
				findings <- makeLinterFinding(param, `*args are not allowed in BUILD files.`)
			case "**":
				findings <- makeLinterFinding(param, `**kwargs are not allowed in BUILD files.`)
			}
		}
	})
}

func printWarning(f *build.File, findings chan *LinterFinding) {
	defer close(findings)
	if f.Type == build.TypeDefault {
		// Only applicable to Bazel files
		return
	}

	build.Walk(f, func(expr build.Expr, stack []build.Expr) {
		call, ok := expr.(*build.CallExpr)
		if !ok {
			return
		}
		ident, ok := (call.X).(*build.Ident)
		if !ok || ident.Name != "print" {
			return
		}
		findings <- makeLinterFinding(expr, `"print()" is a debug function and shouldn't be submitted.`)
	})
}
