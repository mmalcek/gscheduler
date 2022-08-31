protoc ^
	--go_opt=paths=source_relative ^
	--go-grpc_opt=paths=source_relative ^
	--go-grpc_out=go ^
	--go_out=go ^
	--js_out=import_style=commonjs,binary:js ^
	gs.proto