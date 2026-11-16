package chi

import (
	"net/http"

	native "github.com/go-chi/chi/v5"
	"github.com/skolldire/go-engine/router"
)

type Adapter struct {
	mux     *native.Mux
	handler http.Handler
}

func New(cfg router.Config) (*Adapter, error) {
	p, e := router.NewPolicy(cfg)
	if e != nil {
		return nil, e
	}
	mux := native.NewMux()
	return &Adapter{mux: mux, handler: p.Wrap(mux)}, nil
}
func (a *Adapter) Native() *native.Mux   { return a.mux }
func (a *Adapter) Handler() http.Handler { return a.handler }
