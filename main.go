package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"

	"github.com/kenshaw/escpos"
	"github.com/kenshaw/escpos/raster"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/skip2/go-qrcode"
	"golang.org/x/text/encoding/japanese"
)

const printerDevice = "/dev/usb/lp4"

type PrintRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodPost, http.MethodOptions},
	}))

	e.POST("/print", handlePrint)

	e.Logger.Fatal(e.Start(":1323"))
}

func handlePrint(c echo.Context) error {
	var req PrintRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
	}

	if req.Title == "" && req.Body == "" && req.URL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title, body, or url is required"})
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

	if err := printTitle(p, req.Title); err != nil {
		return err
	}

	if err := printBody(p, req.Body); err != nil {
		return err
	}

	if err := printQRCode(p, req.URL); err != nil {
		return err
	}

	p.FormfeedN(3)
	p.Cut()
	p.End()

	if err := rw.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}

	return nil
}

func printTitle(p *escpos.Escpos, title string) error {
	if title == "" {
		return nil
	}

	encoded, err := encodeShiftJIS(title)
	if err != nil {
		return fmt.Errorf("encode title: %w", err)
	}

	p.SetAlign("center")
	p.SetFontSize(2, 2)
	p.SetEmphasize(1)
	p.Write(encoded)
	p.Linefeed()
	p.SetEmphasize(0)
	p.SetFontSize(1, 1)

	return nil
}

func printBody(p *escpos.Escpos, body string) error {
	if body == "" {
		return nil
	}

	encoded, err := encodeShiftJIS(body)
	if err != nil {
		return fmt.Errorf("encode body: %w", err)
	}

	p.SetAlign("left")
	p.Write(encoded)
	p.Linefeed()

	return nil
}

func printQRCode(p *escpos.Escpos, value string) error {
	if value == "" {
		return nil
	}

	qr, err := qrcode.New(value, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("generate QR: %w", err)
	}

	img := qr.Image(256)
	conv := raster.Converter{
		MaxWidth:  576,
		Threshold: 0.5,
	}

	p.SetAlign("center")
	conv.Print(img, p)
	p.Linefeed()

	return nil
}

func encodeShiftJIS(text string) (string, error) {
	encoded, err := japanese.ShiftJIS.NewEncoder().String(text)
	if err != nil {
		return "", err
	}
	return encoded, nil
}
