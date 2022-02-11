package worker

import (
	"fmt"
	"net/http"

	ultraviolet "github.com/realDragonium/Ultraviolet"
)

func NewAPI(backendManager BackendManager) ultraviolet.API {
	return &api{
		backendManager: backendManager,
	}
}

type api struct {
	backendManager BackendManager
	server         http.Server
}

func (api *api) Close() {
	api.server.Close()
}

func (api *api) Run(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/reload", api.reloadHandler)
	api.server = http.Server{Addr: addr, Handler: mux}
	api.server.ListenAndServe()
}

func (api *api) reloadHandler(w http.ResponseWriter, r *http.Request) {
	err := api.backendManager.Update()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(200)
	fmt.Fprintln(w, "success")
}
