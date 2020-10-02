package grpchan

import (
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/grpc"
)

// ServiceRegistry accumulates service definitions. Servers typically have this
// interface for accumulating the services they expose.
type ServiceRegistry interface {
	// RegisterService registers the given handler to be used for the given
	// service. Only a single handler can be registered for a given service. And
	// services are identified by their fully-qualified name (e.g.
	// "package.name.Service"). Attempting to register the same service more
	// than once is an error that can panic.
	RegisterService(desc *grpc.ServiceDesc, srv interface{})
}

var _ ServiceRegistry = (*grpc.Server)(nil)

// HandlerMap is used to accumulate service handlers into a map. The handlers
// can be registered once in the map, and then re-used to configure multiple
// servers that should expose the same handlers. HandlerMap can also be used
// as the internal store of registered handlers for a server implementation.
type handlerMap map[string]service

type HandlerMap struct {
	hm   handlerMap
	hmMu *sync.RWMutex
}

func NewHandlerMap() HandlerMap {
	return HandlerMap{
		hm:   map[string]service{},
		hmMu: &sync.RWMutex{},
	}
}

var _ ServiceRegistry = NewHandlerMap()

type service struct {
	desc    *grpc.ServiceDesc
	handler interface{}
}

// RegisterService registers the given handler to be used for the given service.
// Only a single handler can be registered for a given service. And services are
// identified by their fully-qualified name (e.g. "package.name.Service").
func (r HandlerMap) RegisterService(desc *grpc.ServiceDesc, h interface{}) {
	r.hmMu.Lock()
	defer r.hmMu.Unlock()

	ht := reflect.TypeOf(desc.HandlerType).Elem()
	st := reflect.TypeOf(h)
	if !st.Implements(ht) {
		panic(fmt.Sprintf("service %s: handler of type %v does not satisfy %v", desc.ServiceName, st, ht))
	}
	if _, ok := r.hm[desc.ServiceName]; ok {
		panic(fmt.Sprintf("service %s: handler already registered", desc.ServiceName))
	}
	r.hm[desc.ServiceName] = service{desc: desc, handler: h}
}

// QueryService returns the service descriptor and handler for the named
// service. If no handler has been registered for the named service, then
// nil, nil is returned.
func (r HandlerMap) QueryService(name string) (*grpc.ServiceDesc, interface{}) {
	r.hmMu.RLock()
	defer r.hmMu.RUnlock()

	svc := r.hm[name]
	return svc.desc, svc.handler
}

// ForEach calls the given function for each registered handler. The function is
// provided the service description, and the handler. This can be used to
// contribute all registered handlers to a server and means that applications
// can easily expose the same services and handlers via multiple channels after
// registering the handlers once, with the map:
//
//    // Register all handlers once with the map:
//    reg := channel.NewHandlerMap()
//    // (these registration functions are generated)
//    foo.RegisterHandlerFooBar(newFooBarImpl())
//    fu.RegisterHandlerFuBaz(newFuBazImpl())
//
//    // Now we can re-use these handlers for multiple channels:
//    //   Normal gRPC
//    svr := grpc.NewServer()
//    reg.ForEach(svr.RegisterService)
//    //   In-process
//    ipch := &inprocgrpc.Channel{}
//    reg.ForEach(ipch.RegisterService)
//    //   And HTTP 1.1
//    httpgrpc.HandleServices(http.HandleFunc, "/rpc/", reg, nil, nil)
//
func (r HandlerMap) ForEach(fn func(desc *grpc.ServiceDesc, svr interface{})) {
	r.hmMu.RLock()
	defer r.hmMu.RUnlock()

	for _, svc := range r.hm {
		fn(svc.desc, svc.handler)
	}
}

func (r HandlerMap) Empty() bool {
	r.hmMu.RLock()
	defer r.hmMu.RUnlock()

	return r.hm == nil
}
