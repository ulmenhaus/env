all: python-libraries go-libraries

python-libraries:
	python -m grpc_tools.protoc -I. --python_out=../lib/py/ --grpc_python_out=../lib/py/ jql/jql.proto

go-libraries:
	protoc --go_out=. --go-grpc_out=. jql/jql.proto
