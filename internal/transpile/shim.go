package transpile

// jsrtShim is the JS-style async runtime shim, appended to the generated
// file when async/await/Promise constructs are detected.
//
// Model: each async call runs on its own goroutine; await parks the
// goroutine until the promise settles. This mirrors JS control flow closely
// enough to port logic 1:1, while every piece (jsrtAsync, jsrtAwait, ...) is
// a seam that a later pass can replace with native Go concurrency without
// touching the transpiled call sites.
const jsrtShim = `
// jsrt: minimal JS-style async runtime shim emitted by ts2go.
// Sequential semantics: goroutine-per-async-call, await parks the caller.
type jsrtPromise struct {
	wait chan struct{}
	val  any
	err  error
}

// jsrtTruthy reports JS truthiness: everything is truthy except false, 0,
// "", null, undefined, and NaN.
func jsrtTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && x == x // x == x is false iff NaN
	case int:
		return x != 0
	default:
		return true
	}
}

func jsrtResolve(v any) *jsrtPromise {
	p := &jsrtPromise{wait: make(chan struct{})}
	p.val = v
	close(p.wait)
	return p
}

func jsrtReject(err error) *jsrtPromise {
	p := &jsrtPromise{wait: make(chan struct{})}
	p.err = err
	close(p.wait)
	return p
}

// jsrtAsync runs fn on a new goroutine and returns a promise for its
// result. A panic inside fn rejects the promise (like an uncaught throw).
func jsrtAsync(fn func() any) *jsrtPromise {
	p := &jsrtPromise{wait: make(chan struct{})}
	jsrtPending.Add(1)
	go func() {
		defer jsrtPending.Done()
		defer func() {
			if r := recover(); r != nil {
				p.err = fmt.Errorf("%v", r)
				close(p.wait)
			}
		}()
		p.val = fn()
		close(p.wait)
	}()
	return p
}

// jsrtAwait blocks the calling goroutine until p settles, returning its
// value. A rejected promise re-panics here (like an uncaught throw at the
// await site).
func jsrtAwait(p *jsrtPromise) any {
	<-p.wait
	if p.err != nil {
		panic(p.err)
	}
	return p.val
}

// jsrtAll resolves when every promise settles, in argument order.
func jsrtAll(ps ...*jsrtPromise) *jsrtPromise {
	return jsrtAsync(func() any {
		out := make([]any, 0, len(ps))
		for _, p := range ps {
			out = append(out, jsrtAwait(p))
		}
		return out
	})
}

// jsrtSetTimeout fires fn after ms milliseconds (goroutine-per-timer).
func jsrtSetTimeout(ms float64, fn func()) {
	time.AfterFunc(time.Duration(ms*float64(time.Millisecond)), fn)
}

// jsrtPending tracks in-flight async calls; jsrtRun blocks until quiet.
var jsrtPending sync.WaitGroup

func jsrtRun() { jsrtPending.Wait() }
`
