package main

import (
	"archive/zip"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/chamilad/html-to-markdown"
	"github.com/fatih/color"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// generateSlug generates a filesystem friendly filename from a given post
// title by removing special characters
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

// extractMediumImageStyle reads the image css to extract and translate the
// medium image style to Hugo
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

// downloadFile will download a url to a local file.
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

// fileExists checks if the given file exists
// returns the absolute path if it does
func fileExists(f string) (bool, string) {
	absPath, err := filepath.Abs(f)
	if err != nil {
		return false, ""
	}

	_, err = os.Stat(absPath)
	if err != nil {
		return false, ""
	}

	return true, absPath
}

// https://golangcode.com/unzip-files-in-go/
// Unzip will decompress a zip archive, moving all files and directory
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
			err := os.MkdirAll(fpath, os.ModePerm)
			if err != nil {
				return nil, fmt.Errorf("couldn't extract archive: %s", err)
			}

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
		_ = outFile.Close()
		_ = rc.Close()

		if err != nil {
			return filenames, err
		}
	}

	return filenames, nil
}

// https://www.socketloop.com/tutorials/golang-how-to-tell-if-a-file-is-compressed-either-gzip-or-zip
// isZipFile checks if the given file is of type zip by checking the mime types
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
	return filetype == "application/zip", nil
}

// displayFileName accepts a string and returns either the first 40 chars or
// the string padded up to 40 chars with spaces.
func displayFileName(n string) string {
	nLen := len(n)
	if nLen < 40 {
		paddingChar := " "
		padding := paddingChar
		for ; len(padding)+nLen < 39; {
			padding += paddingChar
		}

		return n + padding
	}

	return n[0:39]
}

// cleanup deletes the medium archive extract
func cleanup(mgr *ConverterManager) {
	if mgr == nil {
		return
	}

	_ = os.RemoveAll(mgr.InPath)
}

// functions used in output tasks ============================================

// printError prints the given string formatted with the subsequent arguments
// to the stdout in red color
func printError(msg string, a ...interface{}) {
	color.Red(msg, a)
}

// printDot prints a dot char to the stdout
func printDot() {
	fmt.Printf("%c", DotMark)
}

// printRedDot prints a red dot to the stdout
func printRedDot() {
	fmt.Printf(color.New(color.FgHiRed).Sprint("%c", DotMark))
}

// printCheckMark prints a unicode check mark to the stdout in green color
func printCheckMark() {
	fmt.Printf(color.New(color.FgHiGreen, color.Bold).Sprintf("%c", CheckMark))
}

// printXMark prints a unicode x mark to the stdout in red color
func printXMark() {
	fmt.Printf(color.New(color.FgHiRed, color.Bold).Sprintf("%c", XMark))
}

// printXError prints a unicode cross mark to the stdout in red
// Used to indicate a failure of a task, the reason for failure is also
// expected as a formattable string
func printXError(msg string, a ...interface{}) {
	fmt.Printf("%s ", color.New(color.BgHiRed, color.FgHiWhite).Sprintf(msg, a))
	printXMark()
}

// functions for bolding text
var boldf = color.New(color.Bold).SprintfFunc()
var bold = color.New(color.Bold).SprintFunc()

// converter rules ============================================================

// converter rule to convert github gists to markdown code blocks
var convertGHGists = md.Rule{
	//<figure name="3f51" id="3f51" class="graf graf--figure graf--iframe graf-after--p"><script src="https://gist.github.com/chamilad/63cfa08c052e795c8e95bb7b43643f6a.js"></script></figure>
	Filter: []string{"script"},
	Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
		codeContentType := ""
		codeContent := ""

		// check the src attribute
		src, exists := selec.Attr("src")
		if !exists {
			// if src cannot be found, nothing can be done
			printRedDot()
			return nil
		}

		// if src exists, check if it is a gist
		if !strings.HasPrefix(src, "https://gist.github") {
			return nil
		}

		// remove the js extension from the path
		// https://gist.github.com/chamilad/63cfa08c052e795c8e95bb7b43643f6a.js
		src = src[0 : len(src)-len(filepath.Ext(src))]
		//fmt.Printf("gist source path: %s", src)

		// get the content type from the html content
		skipTLS := strings.ToLower(os.Getenv("ALLOW_INSECURE")) == "true"
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
		}

		client := &http.Client{Transport: tr}

		res, err := client.Get(src)
		if err != nil {
			printRedDot()
			return nil
		}

		htmlDoc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			printRedDot()
			return nil
		}

		err = res.Body.Close()
		if err != nil {
			printRedDot()
			return nil
		}

		//<meta
		//class="js-ga-set"
		//name="dimension7"
		//content="xml"
		//>
		htmlDoc.Find("meta[name='dimension7']").Each(func(i int, selection *goquery.Selection) {
			ct, exists := selection.Attr("content")
			if !exists {
				printRedDot()
				return
			}

			// certain content types don't translate well to markdown
			if ct == "unknown" {
				codeContentType = ""
			} else if ct == "shell" {
				codeContentType = "bash"
			} else {
				codeContentType = ct
			}
		}) // content type reading done

		// get raw content
		rawsrc := fmt.Sprintf(
			"%s/raw",
			strings.Replace(src, "gist.github.com", "gist.githubusercontent.com", 1))

		rawres, err := client.Get(rawsrc)
		if err != nil {
			printRedDot()
			return nil
		}

		resBody, err := ioutil.ReadAll(rawres.Body)
		if err != nil {
			printRedDot()
			return nil
		}

		err = rawres.Body.Close()
		if err != nil {
			printRedDot()
			return nil
		}

		codeContent = string(resBody) // reading raw content done

		// if no raw content is read, return without rendering
		if len(codeContent) == 0 {
			return nil
		}

		// otherwise render a markdown code block with content type
		codeblock := fmt.Sprintf("\n\n%s%s\n%s\n%s\n\n", options.Fence, codeContentType, codeContent, options.Fence)

		printDot()
		return md.String(codeblock)
	},

	AdvancedReplacement: nil,
}

// convert remaining br tags to new line chars
var convertBreaks = md.Rule{
	Filter: []string{"br"},
	Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
		return md.String("\n")
	},
	AdvancedReplacement: nil,
}

// convert correctly any preformatted sections to unescaped multiline code blocks
// this will also read any pre blocks found as consecutive siblings and collect them into one markdown code
// block.
var convertPre = md.Rule{
	Filter: []string{"pre"},
	Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
		// if this pre tag is already read, skip
		// this happens if pre blocks are found as siblings, the previous pre block processing would collect
		// all the next consecutive pre blocks into one code block and mark them as collected witb this
		// class.
		if selec.HasClass("m2h-collected") {
			return md.String("")
		}

		// read code for current pre block
		codeContent := ""
		readCodeContent(selec, &codeContent)

		// check if next element is a pre block, and read content if so
		nextSelec := selec.Next()
		for ; ; {
			if goquery.NodeName(nextSelec) != "pre" {
				break
			}

			// consecutive blocks usually mean an empty line in medium
			codeContent += "\n\n"

			// append content to single block
			readCodeContent(nextSelec, &codeContent)

			// mark pre block as collected
			nextSelec.AddClass("m2h-collected")

			// check next tag
			nextSelec = nextSelec.Next()
		}

		return md.String(fmt.Sprintf("\n\n%s\n%s\n%s\n\n", options.Fence, codeContent, options.Fence))
	},
	AdvancedReplacement: nil,
}

// readCodeContent reads the text of a given Selection, honouring the br tags found within the text. This is
// intended to be used to read content within pre blocks as the default rule in the converter library will
// ignore the br tags
// The read text will be appended to the string provided by the pointer
func readCodeContent(s *goquery.Selection, c *string) {
	s.Contents().Each(func(i int, selection *goquery.Selection) {
		name := goquery.NodeName(selection)

		if name == "br" {
			*c += "\n"
		} else {
			*c += selection.Text()
		}
	})
}
