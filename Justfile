export GOTOOLCHAIN := "auto"

build:
    go build -o ankibuilder .

run: build
    ./ankibuilder
