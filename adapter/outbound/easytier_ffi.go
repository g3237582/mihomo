//go:build !no_easytier

package outbound

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/component/easytier"
	"github.com/metacubex/mihomo/component/easytierffi"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

// easyTierFFI is one running instance (one data-plane session) shared by
// outbounds that use the same library path and instance-name.
type easyTierFFI struct {
	libPath  string
	instance string
	session  *easytierffi.Session
	refs     int
}

type easyTierFFILib struct {
	native *easytierffi.Native
	refs   int
}

var (
	easyTierFFIMu    sync.Mutex
	easyTierFFIByKey = map[string]*easyTierFFI{}
	easyTierFFILibs  = map[string]*easyTierFFILib{}
)

func ffiKey(libPath, instance string) string {
	return libPath + "\x00" + instance
}

func validateEasyTierFFIOption(option EasyTierOption) error {
	if strings.TrimSpace(option.FFILibrary) == "" {
		return errors.New("missing ffi-library")
	}
	if strings.TrimSpace(option.InstanceName) == "" {
		return errors.New("missing instance-name")
	}
	hasConfig := strings.TrimSpace(option.Config) != ""
	hasFile := strings.TrimSpace(option.ConfigFile) != ""
	if hasConfig == hasFile {
		return errors.New("exactly one of config or config-file is required")
	}
	return nil
}

func ensureTOMLInstanceName(config, instanceName string) string {
	lower := strings.ToLower(config)
	if strings.Contains(lower, "instance_name") || strings.Contains(lower, "inst_name") {
		return config
	}
	return "instance_name = \"" + escapeTOMLString(instanceName) + "\"\n" + config
}

func escapeTOMLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func newEasyTierFFI(option EasyTierOption) (*EasyTier, error) {
	if err := validateEasyTierFFIOption(option); err != nil {
		return nil, err
	}
	libPath := C.Path.Resolve(option.FFILibrary)
	if !C.Path.IsSafePath(libPath) {
		return nil, C.Path.ErrNotSafePath(libPath)
	}
	ctx, cancel := context.WithCancel(context.Background())
	outbound := &EasyTier{
		Base: NewBase(BaseOption{
			Name:         option.Name,
			Addr:         option.InstanceName,
			Type:         C.EasyTier,
			ProviderName: option.ProviderName,
			UDP:          option.UDP,
			Interface:    option.Interface,
			RoutingMark:  option.RoutingMark,
			Prefer:       option.IPVersion,
		}),
		option:     option,
		ctx:        ctx,
		cancel:     cancel,
		ffiLibPath: libPath,
	}
	outbound.dialer = option.NewDialer(outbound.DialOptions())
	return outbound, nil
}

func (e *EasyTier) loadFFIConfig() (string, error) {
	config := e.option.Config
	if strings.TrimSpace(config) == "" {
		path := C.Path.Resolve(e.option.ConfigFile)
		if !C.Path.IsSafePath(path) {
			return "", C.Path.ErrNotSafePath(path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		config = string(data)
	}
	config = ensureTOMLInstanceName(config, e.option.InstanceName)
	return easytier.ApplyRequiredFlags(config), nil
}

func (e *EasyTier) startFFI() error {
	e.startOnce.Do(func() {
		config, err := e.loadFFIConfig()
		if err != nil {
			e.startErr = err
			return
		}
		native, session, err := acquireFFI(e.ffiLibPath, e.option.InstanceName, config)
		if err != nil {
			e.startErr = err
			return
		}
		e.ffi = native
		e.ffiSession = session
		e.startedFFI = true
		log.Infoln("[EasyTier](%s) FFI instance %s started (no_tun)", e.Name(), e.option.InstanceName)
	})
	return e.startErr
}

func acquireFFI(libPath, instance, config string) (*easytierffi.Native, *easytierffi.Session, error) {
	easyTierFFIMu.Lock()
	defer easyTierFFIMu.Unlock()

	lib, ok := easyTierFFILibs[libPath]
	if !ok {
		native, err := easytierffi.Open(libPath)
		if err != nil {
			return nil, nil, err
		}
		lib = &easyTierFFILib{native: native}
		easyTierFFILibs[libPath] = lib
	}
	lib.refs++

	key := ffiKey(libPath, instance)
	if existing := easyTierFFIByKey[key]; existing != nil {
		existing.refs++
		return lib.native, existing.session, nil
	}

	if err := lib.native.RunNetworkInstance(config); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already") {
			releaseFFILibLocked(libPath)
			return nil, nil, err
		}
	}
	session, err := lib.native.OpenSession(instance)
	if err != nil {
		releaseFFILibLocked(libPath)
		return nil, nil, err
	}
	easyTierFFIByKey[key] = &easyTierFFI{
		libPath:  libPath,
		instance: instance,
		session:  session,
		refs:     1,
	}
	return lib.native, session, nil
}

func releaseFFILibLocked(libPath string) {
	lib := easyTierFFILibs[libPath]
	if lib == nil {
		return
	}
	lib.refs--
	if lib.refs > 0 {
		return
	}
	_ = lib.native.Close()
	delete(easyTierFFILibs, libPath)
}

func releaseFFI(libPath, instance string) error {
	key := ffiKey(libPath, instance)
	easyTierFFIMu.Lock()
	shared := easyTierFFIByKey[key]
	if shared == nil {
		easyTierFFIMu.Unlock()
		return nil
	}
	shared.refs--
	if shared.refs > 0 {
		releaseFFILibLocked(libPath)
		easyTierFFIMu.Unlock()
		return nil
	}
	delete(easyTierFFIByKey, key)
	keep := make([]string, 0, len(easyTierFFIByKey))
	for _, item := range easyTierFFIByKey {
		if item.libPath == libPath {
			keep = append(keep, item.instance)
		}
	}
	session := shared.session
	var native *easytierffi.Native
	if lib := easyTierFFILibs[libPath]; lib != nil {
		native = lib.native
	}
	easyTierFFIMu.Unlock()

	var err error
	if session != nil {
		err = session.Close()
	}
	if native != nil {
		if retainErr := native.RetainNetworkInstances(keep); err == nil {
			err = retainErr
		}
	}

	easyTierFFIMu.Lock()
	releaseFFILibLocked(libPath)
	easyTierFFIMu.Unlock()
	return err
}

func (e *EasyTier) dialFFI(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	if err := e.startFFI(); err != nil {
		return nil, err
	}
	if err := e.ResolveUDP(ctx, metadata); err != nil {
		return nil, err
	}
	if e.ffiSession == nil {
		return nil, errors.New("easytier ffi session is not ready")
	}
	conn, err := e.ffiSession.DialContext(ctx, "tcp", metadata.AddrPort().String())
	if err != nil {
		return nil, err
	}
	return NewConn(conn, e), nil
}

func (e *EasyTier) listenFFI(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	if !e.option.UDP {
		return nil, C.ErrNotSupport
	}
	if err := e.startFFI(); err != nil {
		return nil, err
	}
	if err := e.ResolveUDP(ctx, metadata); err != nil {
		return nil, err
	}
	if e.ffiSession == nil {
		return nil, errors.New("easytier ffi session is not ready")
	}
	pc, err := e.ffiSession.ListenPacket(0)
	if err != nil {
		return nil, err
	}
	return NewPacketConn(pc, e), nil
}

func (e *EasyTier) closeFFI() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.startOnce.Do(func() {
		e.startErr = errEasyTierClosed
	})
	var err error
	if e.startedFFI {
		err = releaseFFI(e.ffiLibPath, e.option.InstanceName)
		e.startedFFI = false
		e.ffiSession = nil
	}
	e.ffi = nil
	return err
}
