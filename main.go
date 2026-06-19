package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"fyne.io/systray"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"
)

//go:embed icon.ico
var iconData []byte

var aesKey = []byte("IMSS_OAX_2024_QR")

// ── MessageBox nativo sin consola ─────────────────────────────────────
var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

func msgBox(titulo, mensaje string) {
	t, _ := syscall.UTF16PtrFromString(titulo)
	m, _ := syscall.UTF16PtrFromString(mensaje)
	messageBoxW.Call(0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		0x40)
}

// ── WMI sin PowerShell: leer registro de Windows ─────────────────────
func regRead(path, key string) string {
	if runtime.GOOS != "windows" {
		return "N/A"
	}
	// Usamos syscall directo al registro
	k, err := openRegKey(path)
	if err != nil {
		return "N/A"
	}
	defer syscall.RegCloseKey(k)
	val, err := regQueryString(k, key)
	if err != nil {
		return "N/A"
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "N/A"
	}
	return val
}

func openRegKey(path string) (syscall.Handle, error) {
	var handle syscall.Handle
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	err = syscall.RegOpenKeyEx(
		syscall.HKEY_LOCAL_MACHINE,
		pathPtr,
		0,
		syscall.KEY_READ,
		&handle,
	)
	return handle, err
}

func regQueryString(key syscall.Handle, name string) (string, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", err
	}
	var valType uint32
	var bufSize uint32
	// Primera llamada: obtener tamaño
	syscall.RegQueryValueEx(key, namePtr, nil, &valType, nil, &bufSize)
	if bufSize == 0 {
		return "", fmt.Errorf("empty")
	}
	buf := make([]uint16, bufSize/2+1)
	err = syscall.RegQueryValueEx(key, namePtr, nil, &valType, (*byte)(unsafe.Pointer(&buf[0])), &bufSize)
	if err != nil {
		return "", err
	}
	return syscall.UTF16ToString(buf), nil
}

// ── Autoarranque ─────────────────────────────────────
func configurarAutoarranque() {
    exe, err := os.Executable()
    if err != nil {
        return
    }

    k, err := registry.OpenKey(
        registry.CURRENT_USER,
        `Software\Microsoft\Windows\CurrentVersion\Run`,
        registry.SET_VALUE,
    )
    if err != nil {
        return
    }
    defer k.Close()
    k.SetStringValue("IMSS_TrayApp", exe)
}

// ── hardware ──────────────────────────────────────────────────────────
type HardwareInfo struct {
	Hostname, Username, Domain, IP, MAC, OS, Brand, Model, Serial string
}

type Win32_BIOS struct {
    SerialNumber string
}

func getSerialWMI() string {
    var dst []Win32_BIOS
    err := wmi.Query("SELECT SerialNumber FROM Win32_BIOS", &dst)
    if err != nil || len(dst) == 0 {
        return "N/A"
    }
    s := strings.TrimSpace(dst[0].SerialNumber)
    if s == "" {
        return "N/A"
    }
    return s
}

func getHardwareInfo() HardwareInfo {
	info := HardwareInfo{}
	info.Hostname, _ = os.Hostname()

	info.Username = os.Getenv("USERNAME")
	if info.Username == "" { info.Username = os.Getenv("USER") }
	if info.Username == "" { info.Username = "N/A" }

	info.Domain = os.Getenv("USERDOMAIN")
	if info.Domain == "" { info.Domain = os.Getenv("LOGNAME") }
	if info.Domain == "" { info.Domain = "N/A" }

	info.OS = fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		info.IP = conn.LocalAddr().(*net.UDPAddr).IP.String()
		conn.Close()
	} else {
		info.IP = "N/A"
	}

	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		if len(i.HardwareAddr) > 0 && i.Flags&net.FlagLoopback == 0 {
			info.MAC = strings.ToUpper(i.HardwareAddr.String())
			break
		}
	}
	if info.MAC == "" { info.MAC = "N/A" }

	if runtime.GOOS == "windows" {
		// Leer directo del registro, sin PowerShell ni wmic
		info.Brand  = regRead(`HARDWARE\DESCRIPTION\System\BIOS`, "SystemManufacturer")
		info.Model  = regRead(`HARDWARE\DESCRIPTION\System\BIOS`, "SystemProductName")
		info.Serial = getSerialWMI()
	} else {
		info.Brand  = dmiQuery("system", "Manufacturer")
		info.Model  = dmiQuery("system", "Product Name")
		info.Serial = dmiQuery("system", "Serial Number")
	}
	return info
}

func dmiQuery(tipo, field string) string {
	out, err := exec.Command("sudo", "dmidecode", "-t", tipo).Output()
	if err != nil { return "N/A" }
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, field+":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 { return strings.TrimSpace(parts[1]) }
		}
	}
	return "N/A"
}

// ── cifrado ───────────────────────────────────────────────────────────
func cifrarHex(texto string) string {
	block, err := aes.NewCipher(aesKey)
	if err != nil { return hex.EncodeToString([]byte(texto)) }
	data := []byte(texto)
	bs := block.BlockSize()
	pad := bs - len(data)%bs
	padded := append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ciphertext := make([]byte, bs+len(padded))
	iv := ciphertext[:bs]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return hex.EncodeToString([]byte(texto))
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext[bs:], padded)
	return strings.ToUpper(hex.EncodeToString(ciphertext))
}

// ── QR ────────────────────────────────────────────────────────────────
func generarQR(info HardwareInfo) (string, error) {
	datos := fmt.Sprintf(
		"INVENTARIO IMSS\nHOST: %s\nUSER: %s\nIP: %s | MAC: %s\nSN: %s | MOD: %s",
		info.Hostname, info.Username, info.IP, info.MAC, info.Serial, info.Model,
	)
	ruta := filepath.Join(os.TempDir(), fmt.Sprintf("QR_IMSS_%s.png", info.Hostname))
	err := qrcode.WriteFile(cifrarHex(datos), qrcode.Medium, 256, ruta)
	return ruta, err
}

func abrirImagen(ruta string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", ruta)
	case "darwin":
		cmd = exec.Command("open", ruta)
	default:
		cmd = exec.Command("xdg-open", ruta)
	}
	cmd.Start()
}

func formatoDatos(info HardwareInfo) string {
	return fmt.Sprintf(
		"Equipo  : %s\nUsuario : %s\nDominio : %s\nIP      : %s\nMAC     : %s\nSistema : %s\nMarca   : %s\nModelo  : %s\nSerie   : %s",
		info.Hostname, info.Username, info.Domain,
		info.IP, info.MAC, info.OS,
		info.Brand, info.Model, info.Serial,
	)
}

func notificar(titulo, mensaje string) {
	switch runtime.GOOS {
	case "windows":
		msgBox(titulo, mensaje)
	default:
		exec.Command("notify-send", "-t", "8000", titulo, mensaje).Start()
	}
}

// ── tray ──────────────────────────────────────────────────────────────
func onReady() {
	systray.SetTitle("IMSS")
	systray.SetTooltip("IMSS Oaxaca - Datos del Equipo")
	systray.SetIcon(iconData)

	info := getHardwareInfo()

	mDatos := systray.AddMenuItem("Ver Datos del Equipo", "")
	mQR    := systray.AddMenuItem("Generar Codigo QR", "")
	systray.AddSeparator()
	mSalir := systray.AddMenuItem("Salir", "")

	go func() {
		for {
			select {
			case <-mDatos.ClickedCh:
				notificar("IMSS Oaxaca — Datos del Equipo", formatoDatos(info))
			case <-mQR.ClickedCh:
				ruta, err := generarQR(info)
				if err != nil {
					notificar("Error", "No se pudo generar el QR: "+err.Error())
				} else {
					abrirImagen(ruta)
				}
			case <-mSalir.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() { os.Exit(0) }

func main() {
    if runtime.GOOS == "windows" {
        configurarAutoarranque()
    }
    systray.Run(onReady, onExit)
}