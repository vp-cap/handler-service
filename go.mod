module cap/handler-service

go 1.15

replace cap/handler-service => ./

replace cap/data-lib => ../data-lib

require (
	cap/data-lib v0.0.0-00010101000000-000000000000
	github.com/golang/protobuf v1.4.2
	github.com/spf13/viper v1.7.1
	google.golang.org/grpc v1.32.0
	google.golang.org/protobuf v1.25.0
)
