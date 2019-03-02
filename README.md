# GoHeif - A go gettable decoder/converter for HEIC based on libde265
---

## What is done

- Changes make to @bradfitz's (https://github.com/bradfitz) golang heif parser
  - Some minor bugfixes
  - A few new box parsers, noteably 'iref' and 'hvcC'

- Include libde265's source code (SSE by default enabled) and a simple golang binding

- A Utility `heic2jpg` to illustrate the usage.

## License

- heif and libde265 are in their own licenses

- goheif.go, libde265 golang binding and the `heic2jpg` utility are in MIT license

## Credits
- heif parser by @bradfitz (https://github.com/go4org/go4/tree/master/media/heif)
- libde265 (https://github.com/strukturag/libde265)
- implementation following libheif (https://github.com/strukturag/libheif)

## TODO
- Upstream the changes to heif?


