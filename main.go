package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"net/http"
	"os"
	"strings"

	"github.com/kenshaw/escpos"
	"github.com/kenshaw/escpos/raster"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/yeqown/go-qrcode"
	"golang.org/x/image/draw"
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
	useCenterLogo = false
)

// LayoutType はサポートするレイアウト要素の種類を表す。
type LayoutType string

const (
	LayoutTypeText  LayoutType = "text"
	LayoutTypeLine  LayoutType = "line"
	LayoutTypeQR    LayoutType = "qr"
	LayoutTypeFeed  LayoutType = "feed"
	LayoutTypeSpace LayoutType = "space"
	LayoutTypeCut   LayoutType = "cut"
)

// AlignType はサポートする文字揃えの種類を表す。
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
	echoEcho.Use(
		middleware.CORSWithConfig(
			middleware.CORSConfig{
				AllowOrigins: []string{"*"},
				AllowHeaders: []string{"*"},
				AllowMethods: []string{http.MethodPost, http.MethodOptions},
			},
		),
	)

	echoEcho.POST("/print", handlePrint)

	var err error
	if scheme == "https" {
		err = echoEcho.StartTLS(addr, certFile, keyFile)
	} else {
		err = echoEcho.Start(addr)
	}
	echoEcho.Logger.Fatal(err)
}

func handlePrint(c echo.Context) (
	err error,
) {
	var printRequest PrintRequest
	if err := c.Bind(&printRequest); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{"error": "invalid JSON payload"},
		)
	}

	if len(printRequest.Layout) == 0 {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{"error": "layout is required"},
		)
	}

	if err := printContent(printRequest); err != nil {
		return c.JSON(
			http.StatusInternalServerError,
			map[string]string{"error": err.Error()},
		)
	}

	return c.JSON(
		http.StatusOK,
		map[string]string{"status": "printed"},
	)
}

func printContent(req PrintRequest) (
	err error,
) {
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

	// 一部のプリンターは、デバイスをオープンした直後に最初のバイトを破棄します。
	// 初期化コマンド（ESC @）が切り捨てられて「@」として印刷されないように、
	// 事前に無害なNULLを送信。
	if _, err := rw.Write([]byte{0x00}); err != nil {
		return fmt.Errorf("prime printer: %w", err)
	}
	if err := rw.Flush(); err != nil {
		return fmt.Errorf("prime printer flush: %w", err)
	}

	escposEscpos := escpos.New(rw)
	escposEscpos.Init()
	setJapaneseMode(escposEscpos)

	if err := printLayout(escposEscpos, req.Layout); err != nil {
		return err
	}
	escposEscpos.End()

	if err := rw.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

func printQRCode(
	escposEscpos *escpos.Escpos,
	value string,
) (
	err error,
) {
	if value == "" {
		return
	}

	img, err := buildQRCodeImage(value)
	if err != nil {
		return
	}
	conv := raster.Converter{
		MaxWidth:  576,
		Threshold: 0.5,
	}

	escposEscpos.SetAlign("center")
	conv.Print(img, escposEscpos)
	escposEscpos.Linefeed()

	return
}

func printLayout(
	escposEscpos *escpos.Escpos,
	items []LayoutItem,
) (
	err error,
) {
	for _, item := range items {
		typeName := strings.ToLower(string(item.Type))

		switch typeName {
		case string(LayoutTypeText):
			if err = printLayoutText(escposEscpos, item); err != nil {
				return
			}

		case string(LayoutTypeLine):
			if err = printLayoutLine(escposEscpos); err != nil {
				return
			}

		case string(LayoutTypeQR):
			qrValue := item.QR
			if qrValue == "" {
				qrValue = item.URL
			}
			if qrValue == "" {
				qrValue = item.Text
			}
			if err = printQRCode(escposEscpos, qrValue); err != nil {
				return
			}

		case string(LayoutTypeFeed):
			count := item.Feed
			if count <= 0 {
				count = 1
			}
			escposEscpos.FormfeedN(count)

		case string(LayoutTypeCut):
			escposEscpos.Cut()

		case string(LayoutTypeSpace):
			escposEscpos.Linefeed()

		default:
			return fmt.Errorf("unsupported layout type: %s", item.Type)
		}
	}

	return
}

func printLayoutLine(
	escposEscpos *escpos.Escpos,
) (
	err error,
) {

	encoded, err := encodeShiftJIS(line)
	if err != nil {
		err = fmt.Errorf("encode line: %w", err)
		return
	}

	escposEscpos.SetAlign("center")
	escposEscpos.Write(encoded)

	return
}

func printLayoutText(
	escposEscpos *escpos.Escpos,
	item LayoutItem,
) (
	err error,
) {
	align := strings.ToLower(string(item.Align))
	if align == "" {
		align = string(AlignLeft)
	}
	switch align {
	case string(AlignLeft), string(AlignCenter), string(AlignRight):
		// 許容される値なのでそのまま進む
	default:
		err = fmt.Errorf("unsupported align: %s", item.Align)
		return
	}
	escposEscpos.SetAlign(align)

	width := item.Width
	height := item.Height
	if width == 0 {
		width = 1
	}
	if height == 0 {
		height = 1
	}
	escposEscpos.SetFontSize(width, height)

	if item.Bold {
		escposEscpos.SetEmphasize(1)
	}
	if item.Underline {
		escposEscpos.SetUnderline(1)
	}
	if item.Invert {
		escposEscpos.SetReverse(1)
	}

	if item.Text != "" {
		for _, line := range strings.Split(item.Text, "\n") {
			encoded, err := encodeShiftJIS(line)
			if err != nil {
				return fmt.Errorf("encode text: %w", err)
			}
			escposEscpos.Write(encoded)
			escposEscpos.Linefeed()
		}
	}

	if item.Bold {
		escposEscpos.SetEmphasize(0)
	}
	if item.Underline {
		escposEscpos.SetUnderline(0)
	}
	if item.Invert {
		escposEscpos.SetReverse(0)
	}
	escposEscpos.SetFontSize(1, 1)
	escposEscpos.SetAlign("left")

	return
}

func buildQRCodeImage(
	value string,
) (
	imageImage image.Image,
	err error,
) {

	baseOpts := []qrcode.ImageOption{
		qrcode.WithBorderWidth(3),
		qrcode.WithQRWidth(8),
		qrcode.WithFgColor(color.Black),
		qrcode.WithBgColor(color.White),
		qrcode.WithBuiltinImageEncoder(qrcode.PNG_FORMAT),
	}

	qrc, err := qrcode.New(value, baseOpts...)
	if err != nil {
		return nil, fmt.Errorf("generate QR: %w", err)
	}

	if useCenterLogo {
		logoImg, _ := loadLogoImage()
		if logoImg != nil {
			if attr, attrErr := qrc.Attribute(); attrErr == nil {
				maxLogo := min(attr.W, attr.H) / 5
				logoImg = resizeLogo(logoImg, maxLogo)
				qrc, err = qrcode.New(
					value,
					append(baseOpts, qrcode.WithLogoImage(logoImg))...,
				)
				if err != nil {
					return nil, fmt.Errorf("generate QR with logo: %w", err)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := qrc.SaveTo(&buf); err != nil {
		return nil, fmt.Errorf("render QR: %w", err)
	}

	img, _, err := image.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode QR image: %w", err)
	}

	return img, nil
}

func loadLogoImage() (image.Image, error) {
	const logoPath = "./logo.png"
	f, err := os.Open(logoPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return invertImage(img), nil
}

func resizeLogo(
	imageImage image.Image,
	maxSize int,
) image.Image {
	if imageImage == nil || maxSize <= 0 {
		return imageImage
	}
	width := imageImage.Bounds().Dx()
	height := imageImage.Bounds().Dy()
	if width <= maxSize && height <= maxSize {
		return imageImage
	}

	var newW, newH int
	if width >= height {
		newW = maxSize
		newH = height * maxSize / width
	} else {
		newH = maxSize
		newW = width * maxSize / height
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), imageImage, imageImage.Bounds(), draw.Over, nil)
	return dst
}

func invertImage(
	imageImage image.Image,
) image.Image {
	if imageImage == nil {
		return nil
	}
	bounds := imageImage.Bounds()
	imageRGBA := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := imageImage.At(x, y).RGBA()
			imageRGBA.Set(x, y, color.RGBA{
				R: uint8(255 - r/257),
				G: uint8(255 - g/257),
				B: uint8(255 - b/257),
				A: uint8(a / 257),
			})
		}
	}

	return imageRGBA
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

func setJapaneseMode(
	escposEscpos *escpos.Escpos,
) {
	// ESC/POS プリンタで漢字モードと Shift-JIS を設定。
	// FS & : 漢字モードを有効化
	escposEscpos.WriteRaw([]byte{0x1c, 0x26})
	// FS C n : 漢字コード系の選択 (1 = Shift-JIS)
	escposEscpos.WriteRaw([]byte{0x1c, 0x43, 0x01})
	// ESC R 8 : 日本の国際文字セットを選択（漢字モードと併用）
	escposEscpos.SetLang("ja")
}

func encodeShiftJIS(
	text string,
) (string, error) {
	encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
	if err != nil {
		return "", err
	}
	return encoded, nil
}
