# stego
Go steganography program - hide binary content in PNG image files.

## Example
stego8 can encode binary content into the colour information within a PNG file.  The resultant PNG file will be larger than the original.

### Hiding a file inside a PNG image
Hide secret_file.txt inside test.png and save the resultant image as steg.png:
```shell
go run stego8.go -op encode -i test.png -o steg.png -f secret_file.txt
```

### Extracting a file from a PNG image
Extract secret_file.txt from inside steg.png:
```
go run stego8.go -op decode -i steg.png -f secret_file.txt
```

## TODO
1. Encrypt / decrypt the secret file automatically
2. 
