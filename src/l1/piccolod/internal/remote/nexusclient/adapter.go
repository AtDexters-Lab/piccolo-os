package nexusclient

import "context"

type Client interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    OnStream(handler StreamHandler)
    RegisterPortalListener(listener Listener) error
    RegisterAppListener(app string, listener Listener) error
}

type Listener interface {
    Serve(ctx context.Context, stream Stream) error
}

type Stream interface {
    Context() context.Context
    Close() error
    ProvideLocal(func(ctx context.Context) error) error
    ProxyToLeader(ctx context.Context, leaderAddr string) error
}

type StreamHandler func(stream Stream)
