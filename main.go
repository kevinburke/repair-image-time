package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jdeng/goheif"
	"github.com/kevinburke/handlers"
	"github.com/rwcarlsen/goexif/exif"
)

func main() {
	flag.Usage = func() {
		os.Stderr.WriteString(`fix-image-time dir1 [dir2 dir3...]

Print out all files in the directory that also exist in any other of the
directories.
`)
	}
	max := flag.Int("count", 0, "Stop after processing count entries (default is to not stop)")
	flag.Parse()
	if flag.NArg() < 1 {
		os.Stderr.WriteString("please provide at least one directory.\n\n")
		flag.Usage()
		os.Exit(2)
	}
	for i := range flag.Args() {
		dir := flag.Arg(i)
		count := 0
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if count > *max && (*max > 0) {
				return fs.SkipDir
			}
			ext := filepath.Ext(path)
			switch strings.ToLower(ext) {
			case ".heic":
				fi, err := os.Open(path)
				if err != nil {
					return err
				}
				exifBytes, err := goheif.ExtractExif(fi)
				if err != nil {
					handlers.Logger.Warn("could not extract exif data", "path", path, "err", err)
					return nil
				}
				if err := fi.Close(); err != nil {
					return err
				}
				exifData, err := exif.Decode(bytes.NewReader(exifBytes))
				if err != nil {
					return fmt.Errorf("could not decode exif data from %v: %v", path, err)
				}
				tag, err := exifData.DateTime()
				if err != nil {
					if _, ok := err.(exif.TagNotPresentError); ok {
						// just skip it
						return nil
					}
					fmt.Printf("error getting datetime: %#v\n", err)
					return err
				}
				statT, ok := info.Sys().(*syscall.Stat_t)
				if !ok {
					return fmt.Errorf("could not cast %#v to Stat_t", info.Sys())
				}
				ctime := time.Unix(int64(statT.Birthtimespec.Sec), int64(statT.Birthtimespec.Nsec))
				if tag.IsZero() {
					return nil
				}
				diff := tag.Sub(ctime)
				if diff < 0 {
					diff = -1 * diff
				}
				if diff < 24*time.Hour {
					return nil
				}
				// this will update the "modification time" which, if before
				// the "creation time" will update the creation time to the
				// earlier date, which is the behavior we want. it's not perfect
				// - in theory the image created time could be after the file
				// creation time, in which case it wouldn't update, but this is
				// better than nothing.
				if err := os.Chtimes(path, tag, tag); err != nil {
					return fmt.Errorf("could not update times for %v: %v", path, err)
				}
				handlers.Logger.Info("updated file time", "path", path, "previous_time", ctime, "new_time", tag)
				count++
			}
			if strings.ToLower(ext) != ".jpg" {
				return nil
			}
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
