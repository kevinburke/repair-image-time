package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	pngstructure "github.com/dsoprea/go-png-image-structure/v2"
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
	dryRun := flag.Bool("dry-run", true, "Dry run mode")
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
			lowerext := strings.ToLower(ext)
			var r io.Reader
			fi, err := os.Open(path)
			if err != nil {
				return err
			}
			switch lowerext {
			case ".png":
				parser := pngstructure.NewPngMediaParser()
				ec, parseErr := parser.Parse(fi, int(info.Size()))
				if err := fi.Close(); err != nil {
					return err
				}
				if parseErr != nil {
					handlers.Logger.Warn("could not parse PNG", "path", path, "err", parseErr)
					return nil
				}
				_, rawExif, err := ec.(*pngstructure.ChunkSlice).Exif()
				if err != nil {
					if strings.Contains(err.Error(), "no exif data") {
						// Some photos literally don't have EXIF data, it's okay!
						// handlers.Logger.Info("photo has no EXIF data, skipping", "path", path)
						return nil
					}
					return fmt.Errorf("could not find EXIF data in %q: %w", path, err)
				}
				r = bytes.NewReader(rawExif)
			case ".heic":
				exifBytes, extractErr := goheif.ExtractExif(fi)
				if err := fi.Close(); err != nil {
					return fmt.Errorf("error closing file: %v", err)
				}
				if extractErr != nil {
					handlers.Logger.Warn("could not extract exif data", "path", path, "err", extractErr)
					return nil
				}
				r = bytes.NewReader(exifBytes)
			case ".jpg", ".jpeg":
				parser := jpegstructure.NewJpegMediaParser()
				ec, parseErr := parser.Parse(fi, int(info.Size()+1000))
				if err := fi.Close(); err != nil {
					return err
				}
				if parseErr != nil {
					handlers.Logger.Warn("could not parse JPEG", "path", path, "err", parseErr)
					return nil
				}
				_, segment, err := ec.(*jpegstructure.SegmentList).FindExif()
				if err != nil {
					if strings.Contains(err.Error(), "no exif data") {
						// Some photos literally don't have EXIF data, it's okay!
						// handlers.Logger.Info("photo has no EXIF data, skipping", "path", path)
						return nil
					}
					return fmt.Errorf("could not find EXIF data in %q: %w", path, err)
				}
				_, rawExif, err := segment.Exif()
				if err != nil {
					return fmt.Errorf("could not load exif data: %w", err)
				}
				r = bytes.NewReader(rawExif)
			default:
				fi.Close()
				// unknown file type
				return nil
			}
			exifData, decodeErr := exif.Decode(r)
			if decodeErr != nil {
				handlers.Logger.Warn(fmt.Sprintf("could not decode exif data from %v: %#v", path, decodeErr))
				return nil
			}
			datetime, err := exifData.DateTime()
			if err != nil {
				if _, ok := err.(exif.TagNotPresentError); ok {
					// just skip it
					// handlers.Logger.Info("photo has no datetime info, skipping", "path", path)
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
			if datetime.IsZero() {
				return nil
			}
			diff := datetime.Sub(ctime)
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
			if *dryRun == false {
				if err := os.Chtimes(path, datetime, datetime); err != nil {
					return fmt.Errorf("could not update times for %v: %v", path, err)
				}
			}
			handlers.Logger.Info("updated file time", "dry_run", *dryRun, "path", path, "previous_time", ctime, "new_time", datetime)
			count++
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
