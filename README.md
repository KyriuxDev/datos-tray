# IMSS Oaxaca — Datos del Equipo

Agente ligero de inventario para equipos Windows (con soporte parcial Linux).  
Vive en la bandeja del sistema, consume ~5MB de RAM y no requiere instalación.

---

## ¿Qué hace?

- Muestra datos del equipo desde el tray: hostname, usuario, dominio, IP, MAC, OS, marca, modelo y número de serie
- Genera un código QR con los datos cifrados en AES-128-CBC
- Sin consola, sin ventanas emergentes molestas, sin dependencias externas en el equipo destino

---

## Tecnologías

| Componente | Librería |
|---|---|
| Tray icon | `fyne.io/systray` |
| Código QR | `github.com/skip2/go-qrcode` |
| WMI (número de serie) | `github.com/yusufpapurcu/wmi` |
| Registro de Windows | `golang.org/x/sys/windows/registry` |
| Cifrado AES | `crypto/aes` (stdlib) |
| MessageBox nativo | `user32.dll` via syscall |

---

## Requisitos de desarrollo

- Debian 12/13 (o cualquier Linux con Go)
- Go 1.22+
- `gcc-mingw-w64-x86-64` para cross-compilar a Windows
- `libayatana-appindicator3-dev` para compilar en Linux

---

## Instalación del entorno

### 1. Instalar Go

```bash
wget https://go.dev/dl/go1.26.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 2. Instalar dependencias del sistema

```bash
sudo apt install gcc-mingw-w64-x86-64 libayatana-appindicator3-dev imagemagick
```

### 3. Clonar e inicializar el proyecto

```bash
git clone https://github.com/KyriuxDev/datos-equipo
cd datos-equipo
go mod tidy
```

---

## Dependencias Go

```bash
go get fyne.io/systray
go get github.com/skip2/go-qrcode
go get github.com/yusufpapurcu/wmi
go get golang.org/x/sys/windows/registry
```

---

## Generar el ícono

El ícono se embebe en el binario en tiempo de compilación.  
Debe existir `icon.ico` en la raíz del proyecto.

```bash
# Generar PNG base
mkdir -p genicon
cat > genicon/main.go << 'GOEOF'
package main

import (
    "image"
    "image/color"
    "image/png"
    "os"
)

func main() {
    img := image.NewRGBA(image.Rect(0, 0, 16, 16))
    azul := color.RGBA{0, 102, 204, 255}
    for y := 0; y < 16; y++ {
        for x := 0; x < 16; x++ {
            img.Set(x, y, azul)
        }
    }
    f, _ := os.Create("icon.png")
    png.Encode(f, img)
    f.Close()
}
GOEOF

cd genicon && go run main.go && cd ..
cp genicon/icon.png .

# Convertir a ICO compatible con Windows
convert icon.png -resize 16x16 icon.ico
```

---

## Compilar

### Para Windows (desde Linux)

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags "-H windowsgui" -o IMSS_TrayApp.exe .
```

> `-H windowsgui` evita que aparezca la consola en Windows.  
> `CGO_ENABLED=0` permite cross-compilación sin toolchain nativo de Windows.

### Para Linux

```bash
go build -o imss-tray .
```

---

## Estructura del proyecto

```
datos-equipo/
├── main.go          # Código principal
├── icon.png         # Ícono fuente
├── icon.ico         # Ícono embebido en el binario (Windows)
├── go.mod
├── go.sum
├── genicon/
│   └── main.go      # Genera el ícono PNG
└── README.md
```

---

## Cómo funciona internamente

### Datos del equipo
- **Hostname / Usuario / Dominio** — variables de entorno del OS
- **IP** — conexión UDP a `8.8.8.8` para detectar interfaz activa
- **MAC** — primera interfaz no-loopback via `net.Interfaces()`
- **Marca / Modelo** — registro de Windows: `HKLM\HARDWARE\DESCRIPTION\System\BIOS`
- **Número de serie** — WMI: `Win32_BIOS.SerialNumber` via `yusufpapurcu/wmi`
- **Linux** — `dmidecode` para marca, modelo y serie

### Cifrado del QR
- AES-128-CBC con IV aleatorio
- Clave: `IMSS_OAX_2024_QR` (16 bytes)
- Padding PKCS7
- Resultado en HEX mayúsculas

### Sin consola en Windows
- `MessageBox` — llamada directa a `user32.dll` via `syscall`
- `rundll32 url.dll,FileProtocolHandler` — abre el QR sin consola
- `CGO_ENABLED=0` + `-H windowsgui` — binario GUI puro

---

## Autoarranque en Windows

Para que inicie automáticamente con Windows, agrega el ejecutable al registro:

```
HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run
Nombre: IMSS_TrayApp
Valor:  C:\ruta\al\IMSS_TrayApp.exe
```

O via PowerShell (una sola vez en el equipo destino):

```powershell
$ruta = "C:\ruta\al\IMSS_TrayApp.exe"
Set-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" `
  -Name "IMSS_TrayApp" -Value $ruta
```

---

## Consumo de recursos

| Métrica | Valor |
|---|---|
| RAM en idle | ~5-6 MB |
| RAM Python original | ~28 MB |
| Tamaño del .exe | ~8 MB |
| Tiempo de arranque | ~50 ms |

---

## Comparativa con la versión Python original

| | Python | Go |
|---|---|---|
| RAM | ~28MB | ~5MB |
| Consola al abrir | Sí | No |
| Dependencias en destino | Python + libs | Ninguna |
| Cross-platform | Sí | Sí |
| Tamaño ejecutable | ~35MB | ~8MB |# datos-tray
