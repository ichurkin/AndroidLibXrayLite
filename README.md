# AndroidLibXrayLite

## Build requirements
* JDK
* Android SDK
* Go
* gomobile

## Build instructions
1. `git clone [repo] && cd AndroidLibXrayLite`
2. `gomobile init`
3. `go mod tidy -v`
4. `gomobile bind -v -androidapi 21 -trimpath -ldflags='-s -w -buildid= -checklinkname=0' ./`
better use ./build_using_ndk27d_16kb_support.sh
