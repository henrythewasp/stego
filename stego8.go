// STEGO8 - store 8 bits in each colour byte!
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// Cmd line options
var input_filename = flag.String("i", "", "input image file")
var output_filename = flag.String("o", "", "output image file")
var message_filename = flag.String("f", "", "message input file")
var operation = flag.String("op", "encode", "encode or decode")

// Example encode usage: go run stego8.go -op encode -i test.png -o steg.png -f hide.txt
// Example decode usage: go run stego8.go -op decode -i steg.png -f out.txt

// Bitmask (last 8 bits)
var lsbyte_mask uint32 = ^(uint32(255))

var byte_buffer_len = 256

func panicOnError(e error) {
	if e != nil {
		panic(e)
	}
}

func readImageFile() (image.Image, error) {
	input_reader, err := os.Open(*input_filename)
	if err != nil {
		return nil, err
	}
	defer input_reader.Close()

	img, _, err := image.Decode(input_reader)

	return img, err
}

func encodeRGBA(ch <-chan byte, c uint32) uint32 {
	newc := c
	mb, ok := <-ch
	if ok {
		newc = uint32(mb) + (c & lsbyte_mask)
	}

	return newc
}

func decodeRGBA(c uint32) (uint32, error) {
	return (c & ^lsbyte_mask), nil
}

// ------------------------------------------------------------------------
// Ideas
// + Common up the image-reading code (same in both cases)
// + Can we increase the amount of data hidden - store 4 bits instead of 2 per colour byte? - yes ** stego4.go ** AND stego8.go!!
// + Calculate how much data can be stored in the image ahead of time
// + How about storing the size of the hidden data in the first few image bytes, so we don't have to use an end-marker (0) and we can then hide
//     arbitrary data, and not just ascii text.  Use first byte and use all 4 RGBA colours (can store up to 2GB length)
// + Use a reader for binary input?  Is that more memory efficient than using ioutil.ReadFile?
// + Find out how to get filesize when using a reader (eg for binary file)
// + Write binary output file on decode - don't assume it's ascii text
//
// - Option to spread 4 * 8 bits over the 4 RGBA values - ie. 2 bits of each data byte in R, G, B & A, positionally (ie. instead of 1 value per RGBA value)
// - Make this into a library for re-use
// ------------------------------------------------------------------------

func main() {
	// Parse the command line
	flag.Parse()

	switch *operation {
	case "encode":
		fmt.Println("encoding!")

		fin, input_message_err := os.Open(*message_filename)
		panicOnError(input_message_err)
		defer fin.Close()

		file_info, input_stat_err := os.Stat(*message_filename)
		panicOnError(input_stat_err)
		hidemsg_len := uint32(file_info.Size())
		fmt.Printf("message is %v bytes\n", hidemsg_len)

		// Decode the image
		img, err := readImageFile()
		panicOnError(err)

		// Get the bounds of the image
		bounds := img.Bounds()

		// Check the size of the image to work out how many bytes we can hide
		max_hide_len := uint32(4 * bounds.Dx() * bounds.Dy())
		fmt.Printf("can hide up to %v bytes\n", max_hide_len)

		if max_hide_len < hidemsg_len {
			panicOnError(errors.New("insufficient space in input image."))
		}

		// Create output image
		output_image := image.NewNRGBA64(img.Bounds())

		// Get the rows and columns of the image
		var newr, newg, newb, newa uint32

		fb := make(chan byte) // TODO Perhaps make this buffered (ie. byte_buffer_len?)
		go func() {
			// Read in up to byte_buffer_len bytes at a time
			data := make([]byte, byte_buffer_len)
			for {
				// make a slice
				data = data[:cap(data)]
				n, err := fin.Read(data)
				if err != nil {
					close(fb)
					return
				}
				data = data[:n]
				for _, b := range data {
					fb <- b
				}
			}
		}()

		// Loop over rows
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			// Loop over cols
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// Get the rgba values from the input image (all uint32)
				r, g, b, a := img.At(x, y).RGBA()

				if y == bounds.Min.Y && x == bounds.Min.X {
					// First position.  Store msg len here, in parts
					newr = uint32(hidemsg_len>>24) + (r & lsbyte_mask)
					newg = uint32(hidemsg_len>>16) & ^lsbyte_mask + (g & lsbyte_mask)
					newb = uint32(hidemsg_len>>8) & ^lsbyte_mask + (b & lsbyte_mask)
					newa = uint32(hidemsg_len) & ^lsbyte_mask + (a & lsbyte_mask)

				} else {
					// Message data to hide
					newr = encodeRGBA(fb, r)
					newg = encodeRGBA(fb, g)
					newb = encodeRGBA(fb, b)
					newa = encodeRGBA(fb, a)
				}

				// Store in the image
				output_image.SetNRGBA64(x, y, color.NRGBA64{uint16(newr), uint16(newg), uint16(newb), uint16(newa)})
			}
		}

		// Write the new file out
		output_writer, output_err := os.Create(*output_filename)
		panicOnError(output_err)

		// Close output file when done
		defer output_writer.Close()

		// Encode the png
		png.Encode(output_writer, output_image)

	case "decode":
		// Setup channels for writing decoded message data out
		bo := make(chan uint32) // XXX GT==> Perhaps make this buffered (ie. 256?)
		ex := make(chan int)

		// Anon func to write message out, either to STDOUT or file (if -f opt used)
		go func() {
			var fout *os.File
			var err error

			// Write to the exit channel on completion, to let main goroutine know
			defer func() {
				ex <- 1
			}()

			if *message_filename != "" {
				fmt.Printf("Decoding contents to %v\n", *message_filename)
				fout, err = os.Create(*message_filename)
				panicOnError(err)

				defer fout.Close()
			} else {
				fmt.Printf("Decoding to STDOUT\n")
			}

			buffer := make([]byte, byte_buffer_len)
			position := 0
			for {
				entry, ok := <-bo
				if !ok || (position == byte_buffer_len) {
					if *message_filename != "" {
						fout.Write(buffer[0:position])
					} else {
						fmt.Printf(">> %s", buffer[0:position])
					}
					position = 0

					if !ok {
						return
					}
				}
				buffer[position] = byte(entry)
				position++
			}
		}()

		// Decode the image
		img, err := readImageFile()
		panicOnError(err)

		var hidemsg_len uint32 = 0
		var message_index uint32 = 0

		// Get the bounds of the image
		bounds := img.Bounds()

	OUTER:
		// Loop over rows - break here when finished decoding
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			// Loop over cols
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// Get the rgba values from the input image
				c := img.At(x, y).(color.NRGBA64)

				if y == bounds.Min.Y && x == bounds.Min.X {
					// Build the len from the color bytes
					hidemsg_len = (uint32(c.R) & ^lsbyte_mask) << 24
					hidemsg_len += (uint32(c.G) & ^lsbyte_mask) << 16
					hidemsg_len += (uint32(c.B) & ^lsbyte_mask) << 8
					hidemsg_len += (uint32(c.A) & ^lsbyte_mask)

				} else {
					ch, _ := decodeRGBA(uint32(c.R))
					message_index++
					if message_index > hidemsg_len {
						break OUTER
					}
					bo <- ch

					ch, _ = decodeRGBA(uint32(c.G))
					message_index++
					if message_index > hidemsg_len {
						break OUTER
					}
					bo <- ch

					ch, _ = decodeRGBA(uint32(c.B))
					message_index++
					if message_index > hidemsg_len {
						break OUTER
					}
					bo <- ch

					ch, _ = decodeRGBA(uint32(c.A))
					message_index++
					if message_index > hidemsg_len {
						break OUTER
					}
					bo <- ch
				}
			}
		}

		// Close the binary output channel and wait for goroutine to finish (and flush to output)
		close(bo)
		<-ex
	}
}
