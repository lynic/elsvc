package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/lynic/elsvc"
)

var PluginObj *APIServer

const ModuleName = "svcapi"

type APIServer struct {
	ListenAddr string `json:"listen_addr"`
	ListenPort string `json:"listen_port"`
	router     *mux.Router
	server     *http.Server
	ctx        context.Context
}

func (s APIServer) ModuleName() string {
	return ModuleName
}

func (s *APIServer) Init(ctx context.Context) error {
	conf := elsvc.GetConfig(ctx)
	jdata, _ := json.Marshal(conf)
	err := json.Unmarshal(jdata, s)
	if err != nil {
		return err
	}
	if s.ListenAddr == "" {
		s.ListenAddr = "0.0.0.0"
	}
	if s.ListenPort == "" {
		s.ListenPort = "8989"
	}

	s.router = mux.NewRouter()
	s.router.Handle("/api/v1/message",
		handlers.LoggingHandler(os.Stdout, http.HandlerFunc(s.postMsg))).Methods("POST")
	s.server = &http.Server{
		Addr: fmt.Sprintf("%s:%s", s.ListenAddr, s.ListenPort),
		// Handler: s.server.Router,
		Handler: s.router,
	}

	return nil
}

func (s *APIServer) Start(ctx context.Context) error {
	//store outChan internally
	s.ctx = ctx
	// s.outChan = elsvc.OutChan(ctx)
	// create an error Chan
	errChan := make(chan error, 1)
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
	select {
	case <-ctx.Done():
		s.server.Shutdown(context.Background())
		return nil
	case err := <-errChan:
		return err
	}
}

func (s *APIServer) Stop(context.Context) error {
	return nil
}

func (s *APIServer) postMsg(w http.ResponseWriter, r *http.Request) {
	msg := &elsvc.MsgBase{}
	datas, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": err.Error(),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	err = json.Unmarshal(datas, msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": err.Error(),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	if msg.Type() == "" {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": fmt.Sprintf("invalid msg type: %s", msg.Type()),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	reqMsg := elsvc.NewMsg(elsvc.ChanKeyService, elsvc.MsgListPlugins)
	elsvc.OutChan(s.ctx) <- reqMsg
	if reqMsg.GetError() != nil {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": reqMsg.GetError().Error(),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	plugins := make(map[string]bool)
	err = json.Unmarshal(reqMsg.GetResponseBytes(), &plugins)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": err.Error(),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	if _, ok := plugins[msg.To()]; !ok {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": fmt.Sprintf("plugin %s not available", msg.To()),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	elsvc.OutChan(s.ctx) <- msg
	if msg.GetError() != nil {
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error": msg.GetError().Error(),
		}
		bb, _ := json.Marshal(resp)
		w.Write(bb)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(msg.GetResponseBytes())
}
