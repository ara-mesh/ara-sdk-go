//go:build (linux && (amd64 || arm64)) || (darwin && (amd64 || arm64))

package ara

/*
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/lib/linux_amd64 -laraengine -lm -ldl -lpthread -Wl,-rpath,${SRCDIR}/lib/linux_amd64
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/lib/linux_arm64 -laraengine -lm -ldl -lpthread -Wl,-rpath,${SRCDIR}/lib/linux_arm64
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/lib/darwin_amd64 -laraengine -framework CoreFoundation -framework Security -Wl,-rpath,${SRCDIR}/lib/darwin_amd64
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/lib/darwin_arm64 -laraengine -framework CoreFoundation -framework Security -Wl,-rpath,${SRCDIR}/lib/darwin_arm64
*/
import "C"
