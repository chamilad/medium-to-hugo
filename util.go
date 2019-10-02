package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func generateSlug(s string) string {
	spaces := regexp.MustCompile(`[\s]+`)
	notallowed := regexp.MustCompile(`[^\p{L}\p{N}.\s]`)
	aAndThe := regexp.MustCompile(`^(a\-|the\-)`)

	result := s
	result = strings.Replace(result, "%", " percent", -1)
	result = strings.Replace(result, "#", " sharp", -1)
	result = notallowed.ReplaceAllString(result, "")
	result = spaces.ReplaceAllString(result, "-")
	result = strings.ToLower(result)
	result = aAndThe.ReplaceAllString(result, "")

	return result
}

func extractMediumImageStyle(imgDomElement *goquery.Selection) (mediumImageStyle string) {
	figure := imgDomElement.ParentsUntil("figure.graf").Parent()
	imageStyles := figure.AttrOr("class", "")

	mediumImageLayout := regexp.MustCompile(`graf--(layout\w+)`)
	foundImageLayout := mediumImageLayout.FindStringSubmatch(imageStyles)
	mediumImageStyle = "layoutTextWidth"
	if len(foundImageLayout) > 1 {
		mediumImageStyle = foundImageLayout[1]
	}

	if strings.HasPrefix(mediumImageStyle, "layoutOutsetRow") { // can also be layoutOutsetRowContinue
		imagesInRow := figure.Parent().AttrOr("data-paragraph-count", "")
		mediumImageStyle += imagesInRow
	}

	return
}

// downloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func downloadFile(url, filepath string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func fileExists(f *string) (bool, string) {
	zipFilePath, err := filepath.Abs(*f)
	if err != nil {
		return false, ""
	}
	_, err = os.Stat(zipFilePath)
	if err != nil {
		return false, ""
	}

	return true, zipFilePath
}

func printError(msg string, a ...interface{}) {
	color.Red(msg, a)
}

// Unzip will decompress a zip archive, moving all files and folders
// within the zip file (parameter 1) to an output directory (parameter 2).
func unzipFile(src string, dest string) ([]string, error) {

	var filenames []string
	isZip, err := isZipFile(src)
	if !isZip || err != nil {
		return filenames, errors.New("not a zip archive")
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func isZipFile(f string) (bool, error) {
	zf, err := os.Open(f)
	if err != nil {
		return false, err
	}

	// why 512 bytes ? see http://golang.org/pkg/net/http/#DetectContentType
	buff := make([]byte, 512)

	_, err = zf.Read(buff)
	if err != nil {
		return false, err
	}

	filetype := http.DetectContentType(buff)

	switch filetype {
	case "application/zip":
		return true, nil
	default:
		return false, nil
	}
}
