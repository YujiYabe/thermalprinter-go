package main

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/yeqown/go-qrcode"
	"golang.org/x/text/encoding/japanese"
)

func TestEncodeText(t *testing.T) {
	original := printerEncoding
	defer func() { printerEncoding = original }()

	t.Run("shift-jis", func(t *testing.T) {
		printerEncoding = "shift-jis"
		input := "テスト"

		got, err := encodeText(input)
		if err != nil {
			t.Fatalf("encodeText returned error: %v", err)
		}

		expected, _ := japanese.ShiftJIS.NewEncoder().String(input)
		if got != expected {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	})

	t.Run("utf-8", func(t *testing.T) {
		printerEncoding = "utf-8"
		input := "hello"

		got, err := encodeText(input)
		if err != nil {
			t.Fatalf("encodeText returned error: %v", err)
		}
		if got != input {
			t.Fatalf("expected %q, got %q", input, got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		printerEncoding = "latin-1"

		if _, err := encodeText("text"); err == nil {
			t.Fatal("expected error for unsupported encoding")
		}
	})
}

func TestGetEnvHelpers(t *testing.T) {
	const key = "THERMAL_TEST_KEY"
	const intKey = "THERMAL_TEST_INT"
	const uint8Key = "THERMAL_TEST_UINT8"

	original := os.Getenv(key)
	originalInt := os.Getenv(intKey)
	originalUint8 := os.Getenv(uint8Key)
	defer func() {
		_ = os.Setenv(key, original)
		_ = os.Setenv(intKey, originalInt)
		_ = os.Setenv(uint8Key, originalUint8)
	}()

	_ = os.Unsetenv(key)
	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Fatalf("getEnv returned %q, want fallback", got)
	}

	_ = os.Setenv(key, "value")
	if got := getEnv(key, "fallback"); got != "value" {
		t.Fatalf("getEnv returned %q, want value", got)
	}

	_ = os.Setenv(intKey, "42")
	if got := getEnvInt(intKey, 0); got != 42 {
		t.Fatalf("getEnvInt returned %d, want 42", got)
	}

	_ = os.Setenv(intKey, "not-a-number")
	if got := getEnvInt(intKey, 7); got != 7 {
		t.Fatalf("getEnvInt returned %d, want fallback", got)
	}

	_ = os.Setenv(uint8Key, "0xff")
	if got := getEnvUint8(uint8Key, 0); got != 0xff {
		t.Fatalf("getEnvUint8 returned %d, want 255", got)
	}

	_ = os.Setenv(uint8Key, "invalid")
	if got := getEnvUint8(uint8Key, 9); got != 9 {
		t.Fatalf("getEnvUint8 returned %d, want fallback", got)
	}
}

func TestResizeLogo(t *testing.T) {
	original := image.NewRGBA(image.Rect(0, 0, 200, 100))
	resized := resizeLogo(original, 50)

	if resized.Bounds().Dx() != 50 || resized.Bounds().Dy() != 25 {
		t.Fatalf("unexpected size: %dx%d", resized.Bounds().Dx(), resized.Bounds().Dy())
	}
	if resized == original {
		t.Fatal("resizeLogo should create new image when resizing")
	}

	small := image.NewRGBA(image.Rect(0, 0, 10, 10))
	unchanged := resizeLogo(small, 20)
	if unchanged != small {
		t.Fatal("image within max size should be returned as-is")
	}
}

func TestInvertImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	originalColor := color.RGBA{R: 10, G: 20, B: 30, A: 40}
	img.Set(0, 0, originalColor)

	inverted := invertImage(img)
	result := inverted.At(0, 0)
	r, g, b, a := result.RGBA()

	if r/257 != 245 || g/257 != 235 || b/257 != 225 || a/257 != 40 {
		t.Fatalf("unexpected inverted color: %d %d %d %d", r/257, g/257, b/257, a/257)
	}

	if invertImage(nil) != nil {
		t.Fatal("invertImage(nil) should return nil")
	}
}

func TestBuildQRCodeImage(t *testing.T) {
	img, err := buildQRCodeImage("thermal-printer")
	if err != nil {
		t.Fatalf("buildQRCodeImage returned error: %v", err)
	}

	if img == nil {
		t.Fatal("buildQRCodeImage returned nil image")
	}

	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Fatalf("unexpected image bounds: %v", bounds)
	}
}

func TestConfigureQRErrorCorrectionLevelFromEnv(t *testing.T) {
	const key = "ERROR_CORRECTION_LEVEL"

	originalEnv := os.Getenv(key)
	originalEC := qrErrorCorrectionLevel
	defer func() {
		_ = os.Setenv(key, originalEnv)
		qrErrorCorrectionLevel = originalEC
	}()

	qrErrorCorrectionLevel = qrcode.ErrorCorrectionQuart
	_ = os.Setenv(key, "L")
	configureQRErrorCorrectionLevelFromEnv()
	if qrErrorCorrectionLevel != qrcode.ErrorCorrectionLow {
		t.Fatalf("expected ErrorCorrectionLow, got %v", qrErrorCorrectionLevel)
	}

	qrErrorCorrectionLevel = qrcode.ErrorCorrectionQuart
	_ = os.Setenv(key, "h")
	configureQRErrorCorrectionLevelFromEnv()
	if qrErrorCorrectionLevel != qrcode.ErrorCorrectionHighest {
		t.Fatalf("expected ErrorCorrectionHighest, got %v", qrErrorCorrectionLevel)
	}

	qrErrorCorrectionLevel = qrcode.ErrorCorrectionMedium
	_ = os.Setenv(key, "invalid")
	configureQRErrorCorrectionLevelFromEnv()
	if qrErrorCorrectionLevel != qrcode.ErrorCorrectionMedium {
		t.Fatalf("expected fallback level unchanged, got %v", qrErrorCorrectionLevel)
	}
}
