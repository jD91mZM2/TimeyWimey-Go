default:
	go install
fmt:
	go fmt
	golint
	go vet
race:
	go install --race
