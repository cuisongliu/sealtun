package cmd

import (
	"image"
	"image/color"
	"image/png"
	"net/http"

	"rsc.io/qr"
)

// handleQR renders a PNG QR code for arbitrary short text (a tunnel URL or a
// share URL), so the console can show a scannable code without a JS encoder.
func handleQR(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" || len(text) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required and must be at most 512 chars"})
		return
	}
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	const scale = 8
	const quiet = 4
	size := (code.Size + quiet*2) * scale
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{17, 24, 36, 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, white)
		}
	}
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA((x+quiet)*scale+dx, (y+quiet)*scale+dy, black)
				}
			}
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_ = png.Encode(w, img)
}
