// Package winbind is the platform-free window contract shared by the
// native window files of burnmon and burnmon-dev.
package winbind

import "fmt"

// Window is the subset of the webview API that burnmon uses. Both
// webview2.WebView (go-webview2) and webview.WebView (webview_go) have
// exactly these methods with these signatures.
type Window interface {
	Bind(name string, f interface{}) error
	Dispatch(f func())
	Eval(js string)
	SetHtml(html string)
	Navigate(url string)
	SetTitle(title string)
	Run()
	Terminate()
	Destroy()
}

// Recorder is a Window that only records the names passed to Bind.
type Recorder struct {
	Names []string
}

// Bind records name, or fails when it is already bound.
func (r *Recorder) Bind(name string, f interface{}) error {
	for _, n := range r.Names {
		if n == name {
			return fmt.Errorf("duplicate binding %q", name)
		}
	}
	r.Names = append(r.Names, name)
	return nil
}

func (r *Recorder) Dispatch(f func()) {}
func (r *Recorder) Eval(js string)    {}
func (r *Recorder) SetHtml(h string)  {}
func (r *Recorder) Navigate(u string) {}
func (r *Recorder) SetTitle(t string) {}
func (r *Recorder) Run()              {}
func (r *Recorder) Terminate()        {}
func (r *Recorder) Destroy()          {}
