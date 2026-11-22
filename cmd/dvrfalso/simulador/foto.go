package simulador

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"strconv"
)

var digitos = [10][5]string{
	{"111", "101", "101", "101", "111"}, {"010", "110", "010", "010", "111"},
	{"111", "001", "111", "100", "111"}, {"111", "001", "111", "001", "111"},
	{"101", "101", "111", "001", "001"}, {"111", "100", "111", "001", "111"},
	{"111", "100", "111", "101", "111"}, {"111", "001", "010", "010", "010"},
	{"111", "101", "111", "101", "111"}, {"111", "101", "111", "001", "111"},
}

func guardarFoto(w io.Writer, n int) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	fondo := color.RGBA{uint8(38 + n*57), uint8(76 + n*91), uint8(133 + n*43), 255}
	draw.Draw(img, img.Bounds(), image.NewUniform(fondo), image.Point{}, draw.Src)
	numero := strconv.Itoa(n)
	escala := min(16, 300/(len(numero)*4))
	x := (320 - (len(numero)*4-1)*escala) / 2
	y := (180 - 5*escala) / 2
	blanco := image.NewUniform(color.White)
	for _, d := range numero {
		for fila, l := range digitos[d-'0'] {
			for columna, p := range l {
				if p == '1' {
					x0, y0 := x+columna*escala, y+fila*escala
					draw.Draw(img, image.Rect(x0, y0, x0+escala, y0+escala), blanco, image.Point{}, draw.Src)
				}
			}
		}
		x += 4 * escala
	}
	jpeg.Encode(w, img, nil)
}
