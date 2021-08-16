package api

import (
	"fmt"
	"net/http"

	"github.com/realDragonium/Ultraviolet/server"
)

func NewAPI(backendManager server.BackendManager) API {
	return &api{
		backendManager: backendManager,
	}
}

type API interface {
	Run(addr string)
	Close()
}

type api struct {
	backendManager server.BackendManager
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
