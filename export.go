// Package jsast re-exports esbuild's internal JavaScript parser and AST.
//
// esbuild keeps its parser under internal/, so it is unreachable from any other
// module. This package is the one seam that exposes it: type ALIASES, so
// *jsast.EDot IS *js_ast.EDot and a consumer's type switches work unchanged.
//
// The generated section below is mechanical. The hand-written surface --
// Options, File, Parse -- is deliberately narrow; see options.go for why.
package jsast

import (
	esast "github.com/bytevet/esbuild-jsast/internal/ast"
	"github.com/bytevet/esbuild-jsast/internal/helpers"
	"github.com/bytevet/esbuild-jsast/internal/js_ast"
	"github.com/bytevet/esbuild-jsast/internal/logger"
)

// Position types. logger.Loc.Start is a 0-based BYTE offset into the parsed
// buffer -- not a line/column, and not a rune index.
type (
	Loc   = logger.Loc
	Range = logger.Range
)

// UTF16ToString converts an EString.Value / ETemplate cooked part to a Go
// string. Required, not a convenience: esbuild stores string literals as
// []uint16 because JS strings are UTF-16.
func UTF16ToString(text []uint16) string { return helpers.UTF16ToString(text) }

// ---- generated: js_ast named types ----
type (
	AST                            = js_ast.AST
	AnnotationFlags                = js_ast.AnnotationFlags
	Arg                            = js_ast.Arg
	ArrayBinding                   = js_ast.ArrayBinding
	AssignTarget                   = js_ast.AssignTarget
	B                              = js_ast.B
	BArray                         = js_ast.BArray
	BIdentifier                    = js_ast.BIdentifier
	BMissing                       = js_ast.BMissing
	BObject                        = js_ast.BObject
	Binding                        = js_ast.Binding
	CallKind                       = js_ast.CallKind
	Case                           = js_ast.Case
	Catch                          = js_ast.Catch
	Class                          = js_ast.Class
	ClassStaticBlock               = js_ast.ClassStaticBlock
	ClauseItem                     = js_ast.ClauseItem
	ConstValue                     = js_ast.ConstValue
	ConstValueKind                 = js_ast.ConstValueKind
	Decl                           = js_ast.Decl
	DeclaredSymbol                 = js_ast.DeclaredSymbol
	Decorator                      = js_ast.Decorator
	Dependency                     = js_ast.Dependency
	E                              = js_ast.E
	EAnnotation                    = js_ast.EAnnotation
	EArray                         = js_ast.EArray
	EArrow                         = js_ast.EArrow
	EAwait                         = js_ast.EAwait
	EBigInt                        = js_ast.EBigInt
	EBinary                        = js_ast.EBinary
	EBoolean                       = js_ast.EBoolean
	ECall                          = js_ast.ECall
	EClass                         = js_ast.EClass
	EDot                           = js_ast.EDot
	EFunction                      = js_ast.EFunction
	EIdentifier                    = js_ast.EIdentifier
	EIf                            = js_ast.EIf
	EImportCall                    = js_ast.EImportCall
	EImportIdentifier              = js_ast.EImportIdentifier
	EImportMeta                    = js_ast.EImportMeta
	EImportString                  = js_ast.EImportString
	EIndex                         = js_ast.EIndex
	EInlinedEnum                   = js_ast.EInlinedEnum
	EJSXElement                    = js_ast.EJSXElement
	EJSXText                       = js_ast.EJSXText
	EMissing                       = js_ast.EMissing
	ENameOfSymbol                  = js_ast.ENameOfSymbol
	ENew                           = js_ast.ENew
	ENewTarget                     = js_ast.ENewTarget
	ENull                          = js_ast.ENull
	ENumber                        = js_ast.ENumber
	EObject                        = js_ast.EObject
	EPrivateIdentifier             = js_ast.EPrivateIdentifier
	ERegExp                        = js_ast.ERegExp
	ERequireResolveString          = js_ast.ERequireResolveString
	ERequireString                 = js_ast.ERequireString
	ESpread                        = js_ast.ESpread
	EString                        = js_ast.EString
	ESuper                         = js_ast.ESuper
	ETemplate                      = js_ast.ETemplate
	EThis                          = js_ast.EThis
	EUnary                         = js_ast.EUnary
	EUndefined                     = js_ast.EUndefined
	EYield                         = js_ast.EYield
	EnumValue                      = js_ast.EnumValue
	EqualityKind                   = js_ast.EqualityKind
	ExportStarAlias                = js_ast.ExportStarAlias
	ExportsKind                    = js_ast.ExportsKind
	Expr                           = js_ast.Expr
	Finally                        = js_ast.Finally
	Fn                             = js_ast.Fn
	FnBody                         = js_ast.FnBody
	HelperContext                  = js_ast.HelperContext
	L                              = js_ast.L
	LocalKind                      = js_ast.LocalKind
	ModuleType                     = js_ast.ModuleType
	ModuleTypeData                 = js_ast.ModuleTypeData
	NamedExport                    = js_ast.NamedExport
	NamedImport                    = js_ast.NamedImport
	OpCode                         = js_ast.OpCode
	OpTableEntry                   = js_ast.OpTableEntry
	OptionalChain                  = js_ast.OptionalChain
	Part                           = js_ast.Part
	PrimitiveType                  = js_ast.PrimitiveType
	Property                       = js_ast.Property
	PropertyBinding                = js_ast.PropertyBinding
	PropertyFlags                  = js_ast.PropertyFlags
	PropertyKind                   = js_ast.PropertyKind
	S                              = js_ast.S
	SBlock                         = js_ast.SBlock
	SBreak                         = js_ast.SBreak
	SClass                         = js_ast.SClass
	SComment                       = js_ast.SComment
	SContinue                      = js_ast.SContinue
	SDebugger                      = js_ast.SDebugger
	SDirective                     = js_ast.SDirective
	SDoWhile                       = js_ast.SDoWhile
	SEmpty                         = js_ast.SEmpty
	SEnum                          = js_ast.SEnum
	SExportClause                  = js_ast.SExportClause
	SExportDefault                 = js_ast.SExportDefault
	SExportEquals                  = js_ast.SExportEquals
	SExportFrom                    = js_ast.SExportFrom
	SExportStar                    = js_ast.SExportStar
	SExpr                          = js_ast.SExpr
	SFor                           = js_ast.SFor
	SForIn                         = js_ast.SForIn
	SForOf                         = js_ast.SForOf
	SFunction                      = js_ast.SFunction
	SIf                            = js_ast.SIf
	SImport                        = js_ast.SImport
	SLabel                         = js_ast.SLabel
	SLazyExport                    = js_ast.SLazyExport
	SLocal                         = js_ast.SLocal
	SNamespace                     = js_ast.SNamespace
	SReturn                        = js_ast.SReturn
	SSwitch                        = js_ast.SSwitch
	SThrow                         = js_ast.SThrow
	STry                           = js_ast.STry
	STypeScript                    = js_ast.STypeScript
	SWhile                         = js_ast.SWhile
	SWith                          = js_ast.SWith
	Scope                          = js_ast.Scope
	ScopeKind                      = js_ast.ScopeKind
	ScopeMember                    = js_ast.ScopeMember
	SideEffects                    = js_ast.SideEffects
	Stmt                           = js_ast.Stmt
	StmtsCanBeRemovedIfUnusedFlags = js_ast.StmtsCanBeRemovedIfUnusedFlags
	StrictModeKind                 = js_ast.StrictModeKind
	StringAdditionKind             = js_ast.StringAdditionKind
	SymbolCallUse                  = js_ast.SymbolCallUse
	SymbolUse                      = js_ast.SymbolUse
	TSEnumValue                    = js_ast.TSEnumValue
	TSNamespaceMember              = js_ast.TSNamespaceMember
	TSNamespaceMemberData          = js_ast.TSNamespaceMemberData
	TSNamespaceMemberEnumNumber    = js_ast.TSNamespaceMemberEnumNumber
	TSNamespaceMemberEnumString    = js_ast.TSNamespaceMemberEnumString
	TSNamespaceMemberNamespace     = js_ast.TSNamespaceMemberNamespace
	TSNamespaceMemberProperty      = js_ast.TSNamespaceMemberProperty
	TSNamespaceMembers             = js_ast.TSNamespaceMembers
	TSNamespaceScope               = js_ast.TSNamespaceScope
	TemplatePart                   = js_ast.TemplatePart
)

// ---- generated: ast named types ----
type (
	ImportKind       = esast.ImportKind
	ImportPhase      = esast.ImportPhase
	ImportRecord     = esast.ImportRecord
	Ref              = esast.Ref
	LocRef           = esast.LocRef
	Symbol           = esast.Symbol
	SymbolMap        = esast.SymbolMap
	SymbolKind       = esast.SymbolKind
	ImportItemStatus = esast.ImportItemStatus
	NamespaceAlias   = esast.NamespaceAlias
	Index32          = esast.Index32
)
