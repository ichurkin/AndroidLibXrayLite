# AndroidLibXrayLite

## Build requirements
* JDK
* Android SDK
* Go, use exact go version from go.mod and xray-core to reduce possible runtime errors
* gomobile


## Build instructions
1. `git clone [repo] && cd AndroidLibXrayLite`
2. `gomobile init`
3. `go mod tidy -v`
4. `gomobile bind -v -androidapi 19 -ldflags='-s -w' ./`
