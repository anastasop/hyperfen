
package hyperfen

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"io/ioutil"
	"strings"
)

var (
	iPieces image.Image
	rPieces map[string]image.Point
)

func init() {
	var pgnBytes []byte
	var err error
	if pgnBytes, err = ioutil.ReadFile("pieces/allpieces.png"); err == nil {
		iPieces, err = png.Decode(bytes.NewReader(pgnBytes))
	}
	if err != nil {
		panic(err)
	}

	rPieces = make(map[string]image.Point)
	n := iPieces.Bounds().Dx() / 8
	rPieces["w"] = image.Pt(0, 2).Mul(n)
	rPieces["b"] = image.Pt(0, 3).Mul(n)

	rPieces["wpw"] = image.Pt(0, 6).Mul(n)
	rPieces["wpb"] = image.Pt(1, 6).Mul(n)
	rPieces["wrw"] = image.Pt(7, 7).Mul(n)
	rPieces["wrb"] = image.Pt(0, 7).Mul(n)
	rPieces["wnw"] = image.Pt(1, 7).Mul(n)
	rPieces["wnb"] = image.Pt(6, 7).Mul(n)
	rPieces["wbw"] = image.Pt(5, 7).Mul(n)
	rPieces["wbb"] = image.Pt(2, 7).Mul(n)
	rPieces["wqw"] = image.Pt(3, 7).Mul(n)
	rPieces["wqb"] = image.Pt(3, 4).Mul(n)
	rPieces["wkw"] = image.Pt(4, 4).Mul(n)
	rPieces["wkb"] = image.Pt(4, 7).Mul(n)

	rPieces["bpw"] = image.Pt(1, 1).Mul(n)
	rPieces["bpb"] = image.Pt(0, 1).Mul(n)
	rPieces["brw"] = image.Pt(0, 0).Mul(n)
	rPieces["brb"] = image.Pt(7, 0).Mul(n)
	rPieces["bnw"] = image.Pt(6, 0).Mul(n)
	rPieces["bnb"] = image.Pt(1, 0).Mul(n)
	rPieces["bbw"] = image.Pt(2, 0).Mul(n)
	rPieces["bbb"] = image.Pt(5, 0).Mul(n)
	rPieces["bqw"] = image.Pt(3, 3).Mul(n)
	rPieces["bqb"] = image.Pt(3, 0).Mul(n)
	rPieces["bkw"] = image.Pt(4, 0).Mul(n)
	rPieces["bkb"] = image.Pt(4, 3).Mul(n)
}

func fen2png(fen string) []byte {
	squares := [2]string{"w", "b"}
	pieces := "prnbqk"
	img := image.NewRGBA(iPieces.Bounds())
	n := iPieces.Bounds().Dx() / 8
	ranks := strings.Split(strings.Split(fen, " ")[0] + "////////", "/")[0:8]
	for r := 0; r < 8; r++ {
		rank := (ranks[r] + "********")[0:8]
		f := 0
		for j := 0; j < len(rank); j++ {
			square := (f + (r % 2)) % 2
			piece := rune(rank[j])
			if p := strings.IndexRune("PRNBQK", piece); p != -1 {
				key := "w" + string(pieces[p]) + squares[square]
				draw.Draw(img, image.Rect(f * n, r * n, (f + 1) * n, (r + 1) * n), iPieces, rPieces[key], draw.Over)
				f++
			} else if p := strings.IndexRune("prnbqk", piece); p  != -1 {
				key := "b" + string(pieces[p]) + squares[square]
				draw.Draw(img, image.Rect(f * n, r * n, (f + 1) * n, (r + 1) * n), iPieces, rPieces[key], draw.Over)
				f++
			} else if p := strings.IndexRune("0123456789", piece); p  != -1 {
				for k := 0; k < int(piece - '0'); k++ {
					key := squares[(f + (r % 2)) % 2]
					draw.Draw(img, image.Rect(f * n, r * n, (f + 1) * n, (r + 1) * n), iPieces, rPieces[key], draw.Over)
					f++
				}
			} else {
				key := squares[square]
				draw.Draw(img, image.Rect(f * n, r * n, (f + 1) * n, (r + 1) * n), iPieces, rPieces[key], draw.Over)
				f++
			}
		}
	}

	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}
