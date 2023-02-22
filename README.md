# Koko Tools

This repository is for experiments on GO with GTK4.

There will be some basic tools for everyday use.

## Books
This tool just calculate how many pages would be read in another edition of the book.
## Password generator
Generate random passwords
## Notes
You can now save notes

## Building and running
### Build

*Requirements*
* golang 1.20+
* gtk4
* glib2
* gobject-introspection-1.0

_First build will take more time than others_

```sh
go build -o koko-tools
```
### Run
```sh
./koko-tools
```

If you got error, most likely this will help
```sh
ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH=go1.20 ./koko-tools
```
