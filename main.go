package main

import (
	"bufio"
	"fmt"
	"image"
	"image/draw"
	"net/http"
	"os"
	"strings"

	"github.com/kenshaw/escpos"
	"github.com/kenshaw/escpos/raster"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/skip2/go-qrcode"
	"golang.org/x/text/encoding/japanese"
)

const (
	defaultPrinterDevice = "/dev/usb/lp0"
	defaultCertFile      = "server.crt"
	defaultKeyFile       = "server.key"
)

var (
	printerDevice = defaultPrinterDevice
	certFile      = defaultCertFile
	keyFile       = defaultKeyFile
)

type PrintRequest struct {
	Layout []LayoutItem `json:"layout"`
}

type LayoutItem struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Align     string `json:"align,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Invert    bool   `json:"invert,omitempty"`
	Width     uint8  `json:"width,omitempty"`
	Height    uint8  `json:"height,omitempty"`
	Feed      int    `json:"feed,omitempty"`
	QR        string `json:"qr,omitempty"`
	URL       string `json:"url,omitempty"`
}

func main() {
	loadEnvFile()
	printerDevice = getEnv(
		"PRINTER_DEVICE",
		defaultPrinterDevice,
	)
	certFile = getEnv("ECHO_CERT_FILE", defaultCertFile)
	keyFile = getEnv("ECHO_KEY_FILE", defaultKeyFile)

	port := getEnv("ECHO_PORT", "1323")
	scheme := strings.ToLower(getEnv("ECHO_SCHEME", "http"))
	addr := port
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}

	echoEcho := echo.New()
	echoEcho.HideBanner = true

	echoEcho.Use(middleware.Logger())
	echoEcho.Use(middleware.Recover())
	echoEcho.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{"*"},
	}))

	echoEcho.POST("/print", handlePrint)

	var err error
	if scheme == "https" {
		err = echoEcho.StartTLS(addr, certFile, keyFile)
	} else {
		err = echoEcho.Start(addr)
	}
	echoEcho.Logger.Fatal(err)
}

func handlePrint(c echo.Context) error {
	var req PrintRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
	}

	if len(req.Layout) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "layout is required"})
	}

	if err := printContent(req); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "printed"})
}

func printContent(req PrintRequest) error {
	f, err := os.OpenFile(printerDevice, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open printer: %w", err)
	}
	defer f.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(f), bufio.NewWriter(f))
	p := escpos.New(rw)
	p.Init()
	setJapaneseMode(p)

	if err := printLayout(p, req.Layout); err != nil {
		return err
	}
	p.End()

	if err := rw.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

func printQRCode(p *escpos.Escpos, value string) error {
	if value == "" {
		return nil
	}

	img, err := buildQRCodeImage(value)
	if err != nil {
		return err
	}
	conv := raster.Converter{
		MaxWidth:  576,
		Threshold: 0.5,
	}

	p.SetAlign("center")
	conv.Print(img, p)
	p.Linefeed()

	return nil
}

func printLayout(p *escpos.Escpos, items []LayoutItem) error {
	for _, item := range items {
		typeName := strings.ToLower(item.Type)
		switch typeName {
		case "text", "line":
			if err := printLayoutText(p, item); err != nil {
				return err
			}
		case "qr":
			qrValue := item.QR
			if qrValue == "" {
				qrValue = item.URL
			}
			if qrValue == "" {
				qrValue = item.Text
			}
			if err := printQRCode(p, qrValue); err != nil {
				return err
			}
		case "feed":
			count := item.Feed
			if count <= 0 {
				count = 1
			}
			p.FormfeedN(count)
		case "cut":
			p.Cut()
		case "space":
			p.Linefeed()
		}
	}

	return nil
}

func printLayoutText(p *escpos.Escpos, item LayoutItem) error {
	align := strings.ToLower(item.Align)
	if align == "" {
		align = "left"
	}
	p.SetAlign(align)

	width := item.Width
	height := item.Height
	if width == 0 {
		width = 1
	}
	if height == 0 {
		height = 1
	}
	p.SetFontSize(width, height)

	if item.Bold {
		p.SetEmphasize(1)
	}
	if item.Underline {
		p.SetUnderline(1)
	}
	if item.Invert {
		p.SetReverse(1)
	}

	if item.Text != "" {
		for _, line := range strings.Split(item.Text, "\n") {
			encoded, err := encodeShiftJIS(line)
			if err != nil {
				return fmt.Errorf("encode text: %w", err)
			}
			p.Write(encoded)
			p.Linefeed()
		}
	}

	if item.Bold {
		p.SetEmphasize(0)
	}
	if item.Underline {
		p.SetUnderline(0)
	}
	if item.Invert {
		p.SetReverse(0)
	}
	p.SetFontSize(1, 1)
	p.SetAlign("left")

	return nil
}

func buildQRCodeImage(value string) (image.Image, error) {
	qr, err := qrcode.New(value, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("generate QR: %w", err)
	}

	qr.DisableBorder = true

	const baseSize = 256
	img := qr.Image(baseSize)

	bitmap := qr.Bitmap()
	if len(bitmap) == 0 {
		return nil, fmt.Errorf("empty QR bitmap")
	}

	modulePx := baseSize / len(bitmap)
	if modulePx < 1 {
		modulePx = 1
	}
	quietPx := modulePx // add 1-module quiet zone

	orig, ok := img.(*image.Paletted)
	if !ok {
		return nil, fmt.Errorf("unexpected QR image type %T", img)
	}

	outSize := baseSize + quietPx*2
	out := image.NewPaletted(image.Rect(0, 0, outSize, outSize), orig.Palette)
	draw.Draw(out, image.Rect(quietPx, quietPx, quietPx+baseSize, quietPx+baseSize), orig, image.Point{}, draw.Src)

	return out, nil
}

func loadEnvFile() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}

		_ = os.Setenv(key, val)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func setJapaneseMode(p *escpos.Escpos) {
	// Enable Kanji mode and set Shift-JIS code system on ESC/POS printers.
	// FS & : Enable Kanji
	p.WriteRaw([]byte{0x1c, 0x26})
	// FS C n : Select Kanji code system (1 = Shift-JIS)
	p.WriteRaw([]byte{0x1c, 0x43, 0x01})
	// ESC R 8 : Select Japan international character set (paired with Kanji mode)
	p.SetLang("ja")
}

func encodeShiftJIS(text string) (string, error) {
	encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
	if err != nil {
		return "", err
	}
	return encoded, nil
}
