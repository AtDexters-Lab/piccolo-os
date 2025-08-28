module piccolod

go 1.23.0

toolchain go1.23.11

replace github.com/docker/docker => github.com/moby/moby v26.1.4+incompatible

require (
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/miekg/dns v1.1.68
	golang.org/x/net v0.43.0
	golang.org/x/sys v0.35.0
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
