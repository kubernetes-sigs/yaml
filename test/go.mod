module sigs.k8s.io/yaml/test

go 1.12

require (
	github.com/google/go-cmp v0.5.9
	gopkg.in/yaml.v2 v2.4.0
	sigs.k8s.io/yaml v1.3.0
)

replace sigs.k8s.io/yaml => ../
