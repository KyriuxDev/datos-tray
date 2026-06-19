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
