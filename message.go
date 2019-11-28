package elsvc

import (
	"encoding/json"
	fmt "fmt"
)

const (
	CtxKeyChans   = "chans"
	CtxKeyConfig  = "config"
	CtxKeyInchan  = "in_chan"
	CtxKeyOutchan = "out_chan"
)

const (
	MsgTypeErr  = "msg_error"
	MsgTypeStop = "msg_stop"
)

type MsgBase struct {
	MsgId       string
	MsgFrom     string
	MsgTo       string
	MsgType     string
	MsgRequest  map[string]interface{}
	MsgResponse chan map[string]interface{}
	response    map[string]interface{} // store response for multiple
	TTL         int64
}

func (s MsgBase) ID() string {
	return s.MsgId
}

func (s MsgBase) From() string {
	return s.MsgFrom
}

func (s MsgBase) To() string {
	return s.MsgTo
}

func (s MsgBase) Type() string {
	return s.MsgType
}

func (s MsgBase) GetRequest() map[string]interface{} {
	req := s.MsgRequest
	// tricky to handle two types of plugins
	for k, v := range req {
		switch k {
		// handle error message
		case "error":
			switch errStr := v.(type) {
			case string:
				if errStr == "" {
					req["error"] = nil
				} else {
					req["error"] = fmt.Errorf(errStr)
				}
			}
		}
	}
	return s.MsgRequest
}

func (s MsgBase) GetRequestBytes() []byte {
	req := s.MsgRequest
	// tricky to handle two types of plugins
	for k, v := range req {
		switch k {
		// handle error message
		case "error":
			switch v.(type) {
			case nil:
				req["error"] = ""
			// convert to error
			case error:
				err := v.(error)
				req["error"] = err.Error()
			}
		}
	}
	data, _ := json.Marshal(req)
	return data
}

func (s *MsgBase) SetRequest(req map[string]interface{}) error {
	if req == nil {
		s.MsgRequest = make(map[string]interface{})
		return nil
	}
	s.MsgRequest = req
	//validate by marshal and unmarshal
	_, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return nil
}

func (s *MsgBase) SetRequestBytes(data []byte) error {
	if len(data) == 0 {
		return s.SetRequest(nil)
	}
	req := make(map[string]interface{})
	err := json.Unmarshal(data, &req)
	if err != nil {
		return err
	}
	// tricky to handle two types of plugins
	for k, v := range req {
		switch k {
		// handle error message
		case "error":
			switch v.(type) {
			// convert to error
			case string:
				errStr := v.(string)
				if errStr == "" {
					req["error"] = nil
				} else {
					req["error"] = fmt.Errorf(errStr)
				}
			}
		}
	}
	return s.SetRequest(req)
}

//GetResponse get response
func (s MsgBase) GetResponse() map[string]interface{} {
	if s.response != nil {
		// if GetResponse() called before
		return s.response
	}
	resp := <-s.MsgResponse
	s.response = resp
	// tricky to handle two types of plugins
	for k, v := range resp {
		switch k {
		// handle error message
		case "error":
			switch errStr := v.(type) {
			case string:
				if errStr == "" {
					resp["error"] = nil
				} else {
					resp["error"] = fmt.Errorf(errStr)
				}
			}
		}
	}
	return resp
}

func (s MsgBase) GetResponseBytes() []byte {
	var resp map[string]interface{}
	if s.response != nil {
		// if GetResponse() called before
		resp = s.response
	} else {
		resp = <-s.MsgResponse
		s.response = resp
	}
	if resp == nil {
		// it should never be called
		return []byte("")
	}
	// tricky to handle two types of plugins
	for k, v := range resp {
		switch k {
		// handle error message
		case "error":
			switch v.(type) {
			case nil:
				resp["error"] = ""
			// convert to error
			case error:
				err := v.(error)
				resp["error"] = err.Error()
			}
		}
	}
	data, _ := json.Marshal(resp)
	return data
}

func (s *MsgBase) SetResponse(resp map[string]interface{}) error {
	if resp == nil {
		s.MsgResponse <- make(map[string]interface{})
		return nil
	}
	s.MsgResponse <- resp
	return nil
}

func (s *MsgBase) SetResponseBytes(data []byte) error {
	if len(data) == 0 {
		return s.SetResponse(nil)
	}
	resp := make(map[string]interface{})
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return err
	}
	// tricky to handle two types of plugins
	for k, v := range resp {
		switch k {
		// handle error message
		case "error":
			switch v.(type) {
			// convert to error
			case string:
				errStr := v.(string)
				if errStr == "" {
					resp["error"] = nil
				} else {
					resp["error"] = fmt.Errorf(errStr)
				}
			}
		}
	}
	return s.SetResponse(resp)
}

// func (s *MsgBase) Expired() bool {
// 	return s.TTL <= 0
// }

// func (s *MsgBase) DeTTL() {
// 	s.TTL--
// }
