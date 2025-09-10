# GoHeif - A go gettable decoder/converter for HEIC/AVIF based on libde265/dav1d

## Install
- `heic2jpg` to convert HEIC files to JPG preserving exif 

``` go get github.com/jdeng/goheif/...```

- Tested
  - Mac OS X (High Sierra) 
  - Linux (Ubuntu 16.04 / GCC 5.4)
  - Windows 7 64bit with TDM-GCC 32 (GCC 5.1) and golang 1.12 windows/386

- Code Sample
```
func main() {
	flag.Parse()
	...
  
	fin, fout := flag.Arg(0), flag.Arg(1)
	fi, err := os.Open(fin)
	if err != nil {
		log.Fatal(err)
	}
	defer fi.Close()

	exif, err := goheif.ExtractExif(fi)
	if err != nil {
		log.Printf("Warning: no EXIF from %s: %v\n", fin, err)
	}

	img, err := goheif.Decode(fi)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v\n", fin, err)
	}

	fo, err := os.OpenFile(fout, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("Failed to create output file %s: %v\n", fout, err)
	}
	defer fo.Close()

	w, _ := newWriterExif(fo, exif)
	err = jpeg.Encode(w, img, nil)
	if err != nil {
		log.Fatalf("Failed to encode %s: %v\n", fout, err)
	}

	log.Printf("Convert %s to %s successfully\n", fin, fout)
}
```

## What is done

- Changes make to @bradfitz's (https://github.com/bradfitz) golang heif parser
  - Some minor bugfixes
  - A few new box parsers, noteably 'iref' and 'hvcC'

- Include libde265's source code (SSE by default enabled) and a simple golang binding

- Include dav1d's source code and a simple golang binding

- A Utility `heic2jpg` to illustrate the usage.

## License

- heif and libde265 are in their own licenses
- dav1d is in its own license
- goheif.go, libde265 golang binding and the `heic2jpg` utility are in MIT license

## Credits
- heif parser by @bradfitz (https://github.com/go4org/go4/tree/master/media/heif)
- libde265 (https://github.com/strukturag/libde265)
- dav1d (https://code.videolan.org/videolan/dav1d)
- implementation learnt from libheif (https://github.com/strukturag/libheif)



