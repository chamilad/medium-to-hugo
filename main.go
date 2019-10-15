// A quick script for converting Medium HTML files to Markdown, suitable for use in a static file generator such as Hugo or Jekyll
//A fork of https://gist.github.com/clipperhouse/010d4666892807afee16ba7711b41401
//
// Fork of https://github.com/bgadrian/medium-to-hugo improved for personal preferences
//
package main

// TODO:
//  1. check what happens to gist and other embeds
//  2. check the effect of links inside inline code
//  3. check the effect of quote blocks

import (
	"flag"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chamilad/html-to-markdown"
)

const (
	XMark                 = '\u2718' // unicode char to use for failures
	CheckMark             = '\u2713' // unicode char to use for success
	DotMark               = '\u2022' // unicode bullet chr
	HContentType          = "post"   // Hugo Content Type, only post is supported
	HImagesDirName        = "img"    // directory where the images will be downloaded to
	MarkdownFileExtension = ".md"    // file extension of the Markdown files
	DraftPrefix           = "draft_"

	PostTemplate = `---
title: "{{ .Title }}"
author: "{{ .Author }}"
date: {{ .Date }}
lastmod: {{ .Lastmod }}
{{ if eq .Draft true }}draft: {{ .Draft }}{{end}}
description: "{{ .Description }}"

subtitle: "{{ .Subtitle }}"
{{ if .Tags }}tags:
{{ range .Tags }} - {{.}}
{{end}}{{end}}
{{ if .FeaturedImage }}image: "{{.FeaturedImage}}" {{end}}
{{ if .Images }}images:
{{ range .Images }} - "{{.GetHugoSource}}"
{{end}}{{end}}
{{ if .Canonical }}
aliases:
- "/{{ .Canonical }}"
{{end}}
---

{{ .Body }}
`
)

// A ConverterManager is a structure to collect the details regarding
// the conversion job.
type ConverterManager struct {
	// The path that the export archive will be extracted to
	InPath          string
	MediumPostsPath string // InPath/posts

	// The path the converted markdown files and images will be created in
	OutputPath string
	PostsPath  string // OutputPath/post
	ImagesPath string // OutputPath/post/images

	// Ignore empty articles
	IgnoreEmpty bool
	MDConverter *md.Converter
}

func main() {
	// define input flags
	zipF := flag.String("f", "medium-export.zip", "the medium-export.zip file from Medium")
	ignoreEmpty := flag.Bool("e", false, "ignore empty articles")
	flag.Parse()

	// sanitize and validate input
	exists, zipFilePath := fileExists(*zipF)
	if !exists {
		printError("couldn't find medium-export.zip: %s", zipFilePath)
		os.Exit(1)
	}

	// extract archive and prep for reading
	mgr, err := newConverterManager(zipFilePath, *ignoreEmpty)
	if err != nil {
		printError("error while setting up converter: %s", err)
		cleanup(mgr)
		os.Exit(1)
	}

	// open all the html files for parsing
	files, err := mgr.ReadPosts()
	if err != nil {
		printError("error while reading posts: %s", err)
		cleanup(mgr)
		os.Exit(1)
	}

	fmt.Printf("Posts to process: \t%s\n", boldf("%d", len(files)))

	username, err := mgr.GetMediumUserName()
	if err != nil {
		printError("couldn't read username from archive, self links will not be fixed: %s", err)
	} else {
		fmt.Printf("Medium username: \t%s\n", bold(username))
	}

	fmt.Print("Ignore empty articles: \t", )
	if mgr.IgnoreEmpty {
		printCheckMark()
	} else {
		printXMark()
	}

	fmt.Println()

	// count failures
	ignoreList := make([]string, 0)
	errorList := make([]string, 0)
	successCount := 0

	// iterate each html file and generate md
	for i, f := range files {
		fmt.Printf("\n\t%s: %s => ", boldf("%3d", i+1), displayFileName(f.Name()))

		// if the file is not an html file, ignore
		if !strings.HasSuffix(f.Name(), ".html") || f.IsDir() {
			ignoreList = append(ignoreList, f.Name())
			fmt.Printf("%s ", color.New(color.FgRed).Sprint("ignored, extension"))
			printXMark()
			continue
		}

		printDot()

		fpath := filepath.Join(mgr.MediumPostsPath, f.Name())
		post, err := newPost(fpath)
		if err != nil {
			printXError("reading input => %s", err)
			errorList = append(errorList, f.Name())
			continue
		}

		printDot()

		// cleanup unwanted elements
		post.PruneMediumSpecifics()

		printDot()

		// query dom for interesting values
		post.Date, _ = post.DOM.Find("time").Attr("datetime")
		printDot()
		post.Author = post.DOM.Find(".p-author.h-card").Text()
		printDot()
		post.Title = strings.TrimSpace(post.DOM.Find("title").Text())
		// if title is empty, name it with a random string
		if len(post.Title) == 0 {
			post.Title = fmt.Sprintf("untitled_%s", uuid.New().String())
		}
		printDot()

		subtitle := post.DOM.Find(".p-summary[data-field='subtitle']")
		if subtitle != nil {
			post.Subtitle = strings.TrimSpace(subtitle.Text())
		}
		printDot()

		desc := post.DOM.Find(".p-summary[data-field='description']")
		if desc != nil {
			post.Description = strings.TrimSpace(desc.Text())
		}
		printDot()

		post.SetCanonicalName()
		printDot()

		err = post.FixSelfLinks(username)
		if err != nil {
			printRedDot()
		} else {
			printDot()
		}

		err = post.PopulateTags()
		if err != nil {
			printRedDot()
		} else {
			printDot()
		}

		// determine markdown filename

		// 1. filename prefix, usually <date>_
		//datetime ISO 2018-09-25T14:13:46.823Z
		//we only keep the date for simplicity
		createdDate := strings.Split(post.Date, "T")[0]
		prefix := createdDate

		// drafts get "draft_" as the prefix
		if post.Draft {
			prefix = DraftPrefix
		}
		printDot()

		// 2. clean up the title
		slug := generateSlug(post.Title)
		printDot()

		// 3. collect them to the filename - done
		post.MdFilename = prefix + "_" + slug + MarkdownFileExtension
		printDot()

		// download images
		mgr.ProcessImages(post)
		printDot()

		// all done, generate the markdown
		body := mgr.MDConverter.Convert(post.DOM.Selection)
		post.Body = strings.TrimSpace(body)

		printDot()

		written, err := mgr.Write(post)
		if err != nil {
			printXError("writing output => %s", err)
			errorList = append(errorList, f.Name())
			continue
		}

		// check if an empty body
		if !written {
			ignoreList = append(ignoreList, f.Name())
			fmt.Printf("%s ", color.New(color.FgRed).Sprint("ignored, empty body"))
			printXMark()
		} else {
			printDot()
			successCount++
			fmt.Print(" ")
			printCheckMark()
		}
	}

	fmt.Println()
	fmt.Println()

	if len(ignoreList) > 0 {
		color.Yellow("\n\nThe following files inside posts directory were ignored:")
		for i, ignored := range ignoreList {
			fmt.Printf("%02d: %s\n", i+1, ignored)
		}
	}

	if len(errorList) > 0 {
		color.Red("The following files encountered errors while processing")
		for i, errored := range errorList {
			fmt.Printf("%02d: %s\n", i+1, errored)
		}
	}

	fmt.Println()
	fmt.Println()
	fmt.Printf("%s posts successfully converted to Hugo compatible Markdown\n", bold(successCount))

	fmt.Printf("Output: %s", color.New(color.FgGreen, color.Bold).Sprint(mgr.OutputPath))
	fmt.Println()
	fmt.Println()
	cleanup(mgr)
}

// newConverterManager will create the necessary directories, and extract the
// provided medium export archive in to the InPath
//
// Returns a pointer to the ConverterManager struct, if any failures occur
// during the process, the error will be returned
func newConverterManager(archive string, ignoreEmpty bool) (*ConverterManager, error) {
	// build dir path values
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	t := time.Now()
	oRoot := filepath.Join(
		pwd,
		fmt.Sprintf("medium-to-hugo_%d%02d%02d_%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()))
	oIn := filepath.Join(oRoot, "in")
	oOut := filepath.Join(oRoot, "out")

	// create the directories
	// 1. root and output directories
	err = os.MkdirAll(oRoot, os.ModePerm)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(oOut, os.ModePerm)
	if err != nil {
		return nil, err
	}

	postsPath := filepath.Join(oOut, HContentType)
	imagesPath := filepath.Join(postsPath, HImagesDirName)

	// 2. input dir, unzip will create and extract contents
	files, err := unzipFile(archive, oIn)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("couldn't extract archive: %s => %s", archive, err)
	}

	mediumPosts := filepath.Join(oIn, "posts")
	exists, _ := fileExists(mediumPosts)
	if !exists {
		return nil, fmt.Errorf("couldn't find posts content in the medium extract archive: %s", oIn)
	}

	// create a markdown converter
	op := md.Options{
		CodeBlockStyle: "fenced",
	}
	converter := md.NewConverter("", true, &op)
	// don't remove br tags
	converter.Keep("br")
	converter.AddRules(convertGHGists, convertBreaks, convertPre)

	mgr := &ConverterManager{
		InPath:          oIn,
		MediumPostsPath: mediumPosts,
		OutputPath:      oOut,
		PostsPath:       postsPath,
		ImagesPath:      imagesPath,
		IgnoreEmpty:     ignoreEmpty,
		MDConverter:     converter,
	}

	return mgr, nil
}

// newPost reads a given file and parses the details in to a Post struct.
// The HTML content of the file is also loaded as a queryable reference
// Basic details are extracted from the resulting DOM
//
// If the file cannot be read an error will be returned
func newPost(fullPath string) (*Post, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		return nil, err
	}

	p := &Post{
		DOM:          dom,
		HTMLFileName: filepath.Base(f.Name()),
		Images:       make([]*Image, 0),
		Tags:         make([]string, 0),
		Lastmod:      time.Now().Format(time.RFC3339),
	}

	// draft is prefixed in filename
	p.Draft = strings.HasPrefix(p.HTMLFileName, DraftPrefix)

	return p, nil
}

// GetMediumUserName reads the profile.html file in the extracted medium
// export and extracts the medium username. If an error occur while
// reading the file or the specific element containing the username cannot
// be found, an error will be returned.
func (mgr *ConverterManager) GetMediumUserName() (string, error) {
	//profile/profile.html .u-url
	profileFile := filepath.Join(mgr.InPath, "profile", "profile.html")
	_, err := os.Stat(profileFile)
	if err != nil {
		return "", err
	}

	f, err := os.Open(profileFile)
	if err != nil {
		return "", err
	}

	defer f.Close()

	dom, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		return "", err
	}

	uurlSelection := dom.Find(".u-url")
	if uurlSelection.Length() == 0 {
		return "", fmt.Errorf("couldn't find a username section")
	}

	return strings.TrimPrefix(uurlSelection.First().Text(), "@"), nil
}

// ReadPosts returns a list of FileInfo objects from the extracted medium
// export archive's posts directory. If the files cannot be traversed an
// error will be returned
func (mgr *ConverterManager) ReadPosts() ([]os.FileInfo, error) {
	files, err := ioutil.ReadDir(mgr.MediumPostsPath)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// DownloadImage downloads a given image to the images directory
func (mgr *ConverterManager) DownloadImage(i *Image) error {
	// check if the images directory exists, create if not
	_, err := os.Stat(mgr.ImagesPath)
	if err != nil {
		err = os.MkdirAll(mgr.ImagesPath, os.ModePerm)
		if err != nil {
			return err
		}
	}

	destPath := filepath.Join(mgr.ImagesPath, i.FileName)
	err = downloadFile(i.MediumURL, destPath)
	if err != nil {
		return err
	}

	return nil
}

// Write renders and writes the resulting content into the final markdown file
// inside the output path.
// Returns true if a file was written, false if ignored, error if any errors
// occur
func (mgr *ConverterManager) Write(p *Post) (bool, error) {
	if len(p.Body) == 0 && mgr.IgnoreEmpty {
		return false, nil
	}

	// check if posts path exists, create if not
	_, err := os.Stat(mgr.PostsPath)
	if err != nil {
		err = os.MkdirAll(mgr.PostsPath, os.ModePerm)
		if err != nil {
			return false, err
		}
	}

	f, err := os.Create(filepath.Join(mgr.PostsPath, p.MdFilename))
	if err != nil {
		panic(err)
	}

	defer f.Close()

	tmpl := template.Must(template.New("").Parse(PostTemplate))
	err = tmpl.Execute(f, p)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ProcessImages reads a give Post for img elements, downloads them to a
// directory, and changes the src values to point to the downloaded
// location
func (mgr *ConverterManager) ProcessImages(p *Post) {
	images := p.DOM.Find("img")
	printDot()

	if images.Length() == 0 {
		return
	}

	p.Images = make([]*Image, 0)

	// iterate img elements and process them
	images.Each(func(i int, imgDomElement *goquery.Selection) {
		img, err := p.NewImage(imgDomElement, i)
		if err != nil {
			printRedDot()
			return
		}
		printDot()

		err = mgr.DownloadImage(img)
		if err != nil {
			printRedDot()
			return
		}
		printDot()

		// the suffix after # is useful for styling the image in a way similar to what medium does
		imageSrcAttr := fmt.Sprintf("%s#%s", img.GetHugoSource(), extractMediumImageStyle(imgDomElement))
		imgDomElement.SetAttr("src", imageSrcAttr)
		printDot()

		// if the image is the featured image, mark it down
		if _, isFeatured := imgDomElement.Attr("data-is-featured"); isFeatured {
			p.FeaturedImage = img.GetHugoSource()
		}
		printDot()
	})

	// if no images were marked as featured, get the first image
	if len(p.FeaturedImage) == 0 && len(p.Images) > 0 {
		p.FeaturedImage = p.Images[0].GetHugoSource()
	}
	printDot()

	return
}
