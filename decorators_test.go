package jsast

import (
	"strings"
	"testing"
)

const (
	// A parameter decorator: rejected outright unless ExperimentalDecorators
	// is set. This shape is load-bearing in NestJS and Angular.
	srcParamDecorator = `export class NotificationsController {
  async list(@Req() req: Request) {
    return req.user;
  }
}`

	// Constructor parameters carry both a decorator and a parameter property.
	srcCtorParamDecorator = `export class C {
  constructor(@Optional() x: any, private y: Svc) {}
}`

	// Class, method and property decorators, which parse with the flag off.
	srcClassDecorators = `@Controller('x')
export class C {
  @Get() list() { return 1; }
  @Inject() svc: any;
}`
)

// TestParameterDecoratorNeedsTheFlag pins both halves of the opt-in: off is
// still an error (no silent behaviour change for existing callers), on parses.
func TestParameterDecoratorNeedsTheFlag(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"parameter", srcParamDecorator},
		{"constructor_parameter", srcCtorParamDecorator},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs := Parse(c.src, Options{TS: true})
			if len(errs) == 0 {
				t.Fatal("ExperimentalDecorators off: expected a parse error, got none")
			}
			if !strings.Contains(errs[0].Text, "experimental decorators") {
				t.Errorf("error = %q, want the experimental-decorators diagnostic", errs[0].Text)
			}

			f, errs := Parse(c.src, Options{TS: true, ExperimentalDecorators: true})
			if len(errs) > 0 {
				t.Fatalf("ExperimentalDecorators on: %v", errs)
			}
			if len(f.Stmts) == 0 {
				t.Fatal("no statements")
			}
		})
	}
}

// TestClassDecoratorsParseEitherWay pins the control: the shapes that work
// today must keep working with the flag in both positions.
func TestClassDecoratorsParseEitherWay(t *testing.T) {
	for _, flag := range []bool{false, true} {
		f, errs := Parse(srcClassDecorators, Options{TS: true, ExperimentalDecorators: flag})
		if len(errs) > 0 {
			t.Fatalf("ExperimentalDecorators=%v: %v", flag, errs)
		}
		if len(f.Stmts) == 0 {
			t.Fatalf("ExperimentalDecorators=%v: no statements", flag)
		}
	}
}

// TestExperimentalDecoratorsLowersTheTree makes the cost visible rather than
// incidental. The flag does not merely admit parameter decorators: it lowers
// every experimental decorator in the file, so a caller that reads decorators
// off the tree loses all of them, including on classes and properties that
// parsed fine before. This is the behaviour the Options doc warns about, and
// if upstream ever gains a parse-without-lowering path this test is what will
// notice.
func TestExperimentalDecoratorsLowersTheTree(t *testing.T) {
	asWritten := decoratorShape(t, srcClassDecorators, false)
	if asWritten.classDecorators != 1 || asWritten.propDecorators != 2 {
		t.Errorf("flag off: got %d class and %d property decorators, want 1 and 2",
			asWritten.classDecorators, asWritten.propDecorators)
	}
	if asWritten.runtimeCalls != 0 {
		t.Errorf("flag off: got %d __decorate* calls, want 0 (tree must be source as written)",
			asWritten.runtimeCalls)
	}

	lowered := decoratorShape(t, srcClassDecorators, true)
	if lowered.classDecorators != 0 || lowered.propDecorators != 0 {
		t.Errorf("flag on: got %d class and %d property decorators, want 0 and 0 (lowering strips them)",
			lowered.classDecorators, lowered.propDecorators)
	}
	if lowered.runtimeCalls == 0 {
		t.Error("flag on: expected __decorateClass/__decorateParam calls in the lowered tree")
	}
}

type shape struct {
	classDecorators, propDecorators, runtimeCalls int
}

// decoratorShape counts decorators still attached to the tree, and calls to
// the injected decorator runtime, using only this package's public surface.
func decoratorShape(t *testing.T, src string, experimental bool) shape {
	t.Helper()
	f, errs := Parse(src, Options{TS: true, ExperimentalDecorators: experimental})
	if len(errs) > 0 {
		t.Fatalf("Parse(ExperimentalDecorators=%v): %v", experimental, errs)
	}

	var s shape
	f.Inspect(func(n Node) bool {
		switch d := n.Data.(type) {
		case *SClass:
			s.classDecorators += len(d.Class.Decorators)
			for _, prop := range d.Class.Properties {
				s.propDecorators += len(prop.Decorators)
			}
		case *ECall:
			if id, ok := d.Target.Data.(*EIdentifier); ok {
				if strings.HasPrefix(f.NameOf(id.Ref), "__decorate") {
					s.runtimeCalls++
				}
			}
		}
		return true
	})
	return s
}
