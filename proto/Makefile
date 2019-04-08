all:
	capnp compile -ogo \
	-I $(GOPATH)/src/github.com/glycerine/go-capnproto proto.capnp
	
fmt:
	./capfmt proto.capnp
