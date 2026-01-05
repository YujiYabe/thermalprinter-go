package main

import (
	"bufio"
	"errors"
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

// LayoutType defines supported layout element kinds.
type LayoutType string

const (
	LayoutTypeText  LayoutType = "text"
	LayoutTypeLine  LayoutType = "line"
	LayoutTypeQR    LayoutType = "qr"
	LayoutTypeFeed  LayoutType = "feed"
	LayoutTypeSpace LayoutType = "space"
	LayoutTypeCut   LayoutType = "cut"
)

// AlignType defines supported text alignment values.
type AlignType string

const (
	AlignLeft   AlignType = "left"
	AlignCenter AlignType = "center"
	AlignRight  AlignType = "right"
)

const line = "＿＿＿＿＿＿＿＿＿＿＿＿＿" + "\n\n"

type PrintRequest struct {
	Layout []LayoutItem `json:"layout"`
}

type LayoutItem struct {
	Type      LayoutType `json:"type"`
	Text      string     `json:"text,omitempty"`
	Align     AlignType  `json:"align,omitempty"`
	Bold      bool       `json:"bold,omitempty"`
	Underline bool       `json:"underline,omitempty"`
	Invert    bool       `json:"invert,omitempty"`
	Width     uint8      `json:"width,omitempty"`
	Height    uint8      `json:"height,omitempty"`
	Feed      int        `json:"feed,omitempty"`
	QR        string     `json:"qr,omitempty"`
	URL       string     `json:"url,omitempty"`
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
	var printRequest PrintRequest
	if err := c.Bind(&printRequest); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
	}

	if len(printRequest.Layout) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "layout is required"})
	}

	if err := printContent(printRequest); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "printed"})
}

func printContent(req PrintRequest) error {
	osFile, err := os.OpenFile(printerDevice, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			if chmodErr := os.Chmod(printerDevice, 0o666); chmodErr != nil {
				return fmt.Errorf("change printer permission: %w", chmodErr)
			}
			osFile, err = os.OpenFile(printerDevice, os.O_RDWR, 0)
		}
		if err != nil {
			return fmt.Errorf("open printer: %w", err)
		}
	}
	defer osFile.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(osFile), bufio.NewWriter(osFile))

	// Some printers drop the first byte right after the device is opened.
	// Send a harmless NUL up-front so the initialization command (ESC @)
	// is not truncated and printed as a literal '@'.
	if _, err := rw.Write([]byte{0x00}); err != nil {
		return fmt.Errorf("prime printer: %w", err)
	}
	if err := rw.Flush(); err != nil {
		return fmt.Errorf("prime printer flush: %w", err)
	}

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
		typeName := strings.ToLower(string(item.Type))

		switch typeName {
		case string(LayoutTypeText):
			if err := printLayoutText(p, item); err != nil {
				return err
			}

		case string(LayoutTypeLine):
			if err := printLayoutLine(p); err != nil {
				return err
			}

		case string(LayoutTypeQR):
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

		case string(LayoutTypeFeed):
			count := item.Feed
			if count <= 0 {
				count = 1
			}
			p.FormfeedN(count)

		case string(LayoutTypeCut):
			p.Cut()

		case string(LayoutTypeSpace):
			p.Linefeed()

		default:
			return fmt.Errorf("unsupported layout type: %s", item.Type)
		}
	}

	return nil
}

func printLayoutLine(p *escpos.Escpos) error {

	encoded, err := encodeShiftJIS(line)
	if err != nil {
		return fmt.Errorf("encode line: %w", err)
	}

	p.SetAlign("center")
	p.Write(encoded)

	return nil
}

func printLayoutText(p *escpos.Escpos, item LayoutItem) error {
	align := strings.ToLower(string(item.Align))
	if align == "" {
		align = string(AlignLeft)
	}
	switch align {
	case string(AlignLeft), string(AlignCenter), string(AlignRight):
		// ok
	default:
		return fmt.Errorf("unsupported align: %s", item.Align)
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
