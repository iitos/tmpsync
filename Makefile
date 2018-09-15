all: clean build

clean:
	@echo "## rm tmpsync"
	@rm -f tmpsync

build:
	@echo "## build tmpsync"
	@go build -ldflags "-extldflags=-Wl,--allow-multiple-definition" .

style:
	@echo "## style tmpsync"
	@gofmt -w .

getall:
	@echo "## get all dependencies"
	@go get -d .
