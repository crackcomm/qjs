package qjs

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

// stackFrameRe matches a single line of a QuickJS stack trace, for example:
//
//	at outer (outer.js:2:23)
//	at without-barrier.js:1:1
//
// stackFrameRe matches a single line of a QuickJS stack trace, for example:
//
//	at outer (outer.js:2:23)
//	at <eval> (test.js:21:1)
//	at test.js:1:1
//
// Capture groups: function name (optional), file, line, column. The regex is
// multiline-anchored so it matches frames anywhere in the trace (not just
// those preceded by a newline), and the function name group is optional so
// anonymous top-level frames are also parsed.
var stackFrameRe = regexp.MustCompile(`(?m)^\s*at\s+(?:(.+?)\s+\()?(.+?):(\d+):(\d+)\)?`)

// StackFrame is a single frame of a JavaScript stack trace.
type StackFrame struct {
	// Function is the name of the function for this frame. It is empty for
	// top-level (anonymous) frames.
	Function string
	// File is the source file (or eval name) the frame originates from.
	File string
	// Line is the 1-based line number within File.
	Line int
	// Col is the 1-based column number within File.
	Col int
}

// String renders the frame as "file:line:col".
func (f StackFrame) String() string {
	return fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Col)
}

// JSError represents an error thrown by JavaScript code. It captures the error
// type, message and, when available, the raw stack trace along with its parsed
// frames.
type JSError struct {
	// Type is the error constructor name (e.g. "TypeError"). It defaults to
	// "Error" when the thrown value does not provide one.
	Type string
	// Message is the error message.
	Message string
	// Stack is the raw stack trace string as reported by the engine, or empty
	// when unavailable.
	Stack string
	// Frames holds the parsed frames of Stack, in order from innermost to
	// outermost.
	Frames []StackFrame
}

// Error implements the error interface. It returns the error headline followed
// by the raw stack trace when one is present.
func (e *JSError) Error() string {
	if e.Stack == "" {
		return e.headline()
	}
	return e.headline() + "\n" + e.Stack
}

// headline returns the single-line summary of the error, including the location
// of the innermost frame when available.
func (e *JSError) headline() string {
	if len(e.Frames) > 0 {
		return fmt.Sprintf("%s: %s at %s", e.Type, e.Message, e.Frames[0])
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// newJSError builds a JSError from a thrown JavaScript value.
func newJSError(v *Value) *JSError {
	name := v.GetPropertyStr("name")
	defer name.Free()
	message := v.GetPropertyStr("message")
	defer message.Free()
	stack := v.GetPropertyStr("stack")
	defer stack.Free()

	e := &JSError{
		Type:    trimUndefined(name.String()),
		Message: trimUndefined(message.String()),
	}

	// Fallback for non-Error throws, e.g. `throw "boom"` or `throw 42`.
	if e.Type == "" && e.Message == "" {
		e.Message = v.String()
	}
	if e.Type == "" {
		e.Type = "Error"
	}

	if !stack.IsUndefined() {
		e.Stack = stack.String()
		e.Frames = parseStack(e.Stack)
	}

	return e
}

// trimUndefined maps the JavaScript "undefined" string to an empty string.
func trimUndefined(s string) string {
	if s == "undefined" {
		return ""
	}
	return s
}

// parseStack extracts the frames from a raw stack trace string.
func parseStack(stack string) []StackFrame {
	matches := stackFrameRe.FindAllStringSubmatch(stack, -1)
	if len(matches) == 0 {
		return nil
	}

	frames := make([]StackFrame, 0, len(matches))
	for _, m := range matches {
		line, _ := strconv.Atoi(m[3])
		col, _ := strconv.Atoi(m[4])
		frames = append(frames, StackFrame{
			Function: strings.TrimSpace(m[1]),
			File:     m[2],
			Line:     line,
			Col:      col,
		})
	}
	return frames
}

var (
	ErrRType                   = reflect.TypeOf((*error)(nil)).Elem()
	ErrZeroRValue              = reflect.Zero(ErrRType)
	ErrCallFuncOnNonObject     = errors.New("cannot call function on non-object")
	ErrNotAnObject             = errors.New("value is not an object")
	ErrObjectNotAConstructor   = errors.New("object not a constructor")
	ErrInvalidFileName         = errors.New("file name is required")
	ErrMissingProperties       = errors.New("value has no properties")
	ErrInvalidPointer          = errors.New("null pointer dereference")
	ErrIndexOutOfRange         = errors.New("index out of range")
	ErrNoNullTerminator        = errors.New("no NUL terminator")
	ErrInvalidContext          = errors.New("invalid context")
	ErrNotANumber              = errors.New("js value is not a number")
	ErrAsyncFuncRequirePromise = errors.New("jsFunctionProxy: async function requires a promise")
	ErrEmptyStringToNumber     = errors.New("empty string cannot be converted to number")
	ErrJsFuncDeallocated       = errors.New("js function context has been deallocated")
	ErrNotByteArray            = errors.New("invalid TypedArray: buffer is not a byte array")
	ErrNotArrayBuffer          = errors.New("input is not an ArrayBuffer")
	ErrMissingBufferProperty   = errors.New("invalid TypedArray: missing buffer property")
	ErrRuntimeClosed           = errors.New("runtime is closed")
	ErrNilModule               = errors.New("WASM module is nil")
	ErrNilHandle               = errors.New("handle is nil")
	ErrChanClosed              = errors.New("channel is closed")
	ErrChanSend                = errors.New("channel send would block: buffer full or no receiver ready")
	ErrChanReceive             = errors.New("channel receive would block: buffer empty or no sender ready")
	ErrChanCloseReceiveOnly    = errors.New("cannot close receive-only channel")
)

func combineErrors(errs ...error) error {
	if len(errs) == 0 {
		return nil
	}

	var errStr string

	for _, err := range errs {
		if err != nil {
			errStr += err.Error() + "\n"
		}
	}

	return errors.New(errStr)
}

func newMaxLengthExceededErr(request uint, maxLen int64, index int) error {
	return fmt.Errorf("length %d exceeds max %d at index %d", request, maxLen, index)
}

func newOverflowErr(value any, targetType string) error {
	return fmt.Errorf("value %v overflows %s", value, targetType)
}

func newGoToJsErr(kind string, err error, details ...string) error {
	detail := ""
	if len(details) > 0 {
		detail = " " + details[0]
	}

	if err == nil {
		return fmt.Errorf("cannot convert Go%s '%s' to JS", detail, kind)
	}

	return fmt.Errorf("cannot convert Go%s '%s' to JS: %w", detail, kind, err)
}

func newJsToGoErr(kind *Value, err error, details ...string) error {
	detail := ""
	if len(details) > 0 {
		detail = " " + details[0]
	}

	kindStr := ""

	var kindErr error

	if kind != nil {
		kindStr, kindErr = kind.JSONStringify()
		if kindErr != nil {
			kindStr = fmt.Errorf("(%w), %s", kindErr, kind.String()).Error()
		}
	}

	if kindStr == "undefined" || kindStr == "null" {
		kindStr = kind.Type()
	}

	if kindStr != "" {
		kindStr = " " + kindStr
	}

	if err == nil {
		return fmt.Errorf("cannot convert JS%s%s to Go", detail, kindStr)
	}

	return fmt.Errorf("cannot convert JS%s%s to Go: %w", detail, kindStr, err)
}

func newArgConversionErr(index int, err error) error {
	return fmt.Errorf("cannot convert JS function argument at index %d: %w", index, err)
}

func newInvalidGoTypeErr(expect string, got any) error {
	return fmt.Errorf("expected GO type %s, got %T", expect, got)
}

func newInvalidJsInputErr(kind string, input *Value) (err error) {
	var detail string
	if detail, err = input.JSONStringify(); err != nil {
		detail = fmt.Sprintf("(JSONStringify failed: %v), (.String()) %s", err, input.String())
	}

	return fmt.Errorf("expected JS %s, got %s=%s", kind, input.Type(), detail)
}

func newJsStringifyErr(kind string, err error) error {
	return fmt.Errorf("js %s: %w", kind, err)
}

func newProxyErr(id uint64, r any) error {
	if err, ok := r.(error); ok {
		return fmt.Errorf("functionProxy [%d]: %w\n%s", id, err, debug.Stack())
	}

	if str, ok := r.(string); ok {
		return fmt.Errorf("functionProxy [%d]: %s\n%s", id, str, debug.Stack())
	}

	return fmt.Errorf("functionProxy [%d]: %v\n%s", id, r, debug.Stack())
}

func newInvokeErr(input *Value, err error) error {
	return fmt.Errorf("cannot call getTime on JS value '%s', err=%w", input.String(), err)
}
