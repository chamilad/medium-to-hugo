// A quick script for converting Medium HTML files to Markdown, suitable for use in a static file generator such as Hugo or Jekyll
//A fork of https://gist.github.com/clipperhouse/010d4666892807afee16ba7711b41401
package main

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
)

const (
	XMark                 = '\u2718'
	CheckMark             = '\u2713'
	HContentType          = "post" // Hugo Content Type, only post is supported
	HImagesDirName        = "img"
	MarkdownFileExtension = ".md"
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

// TODO:
//   2. figure out username from medium extract - profile/profile.html .u-url
//   3. flag to create subfolders for posts

type ConverterManager struct {
	InPath, MediumPostsPath, OutputPath, PostsPath, ImagesPath string
}

func main() {
	// define input flags
	zipF := flag.String("f", "medium-export.zip", "the medium-export.zip file from Medium")
	flag.Parse()

	// sanitize and validate input
	exists, zipFilePath := fileExists(*zipF)
	if !exists {
		printError("couldn't find medium-export.zip: %s", zipFilePath)
		os.Exit(1)
	}

	mgr, err := newConverterManager(zipFilePath)
	if err != nil {
		printError("error while setting up converter: %s", err)
		cleanup(mgr)
		os.Exit(1)
	}

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

	fmt.Println()

	ignoreList := make([]string, 0)
	errorList := make([]string, 0)
	successCount := 0

	// iterate each html file and generate md
	for i, f := range files {
		fmt.Printf("\n\t%s: %s => ", boldf("%02d", i+1), displayFileName(f.Name()))

		if !strings.HasSuffix(f.Name(), ".html") || f.IsDir() {
			//fmt.Printf("Ignoring (ext) %s\n", f.Name())
			ignoreList = append(ignoreList, f.Name())
			printXMark("ignored")
			continue
		}

		printDot()

		fpath := filepath.Join(mgr.MediumPostsPath, f.Name())
		post, err := newPost(fpath)
		if err != nil {
			printXMark("reading input => %s", err)
			errorList = append(errorList, f.Name())
			//printError("error while reading html file: %s => %s", f.Name(), err)
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

		//datetime ISO 2018-09-25T14:13:46.823Z
		//we only keep the date for simplicity
		createdDate := strings.Split(post.Date, "T")[0]
		prefix := createdDate

		if post.Draft {
			prefix = DraftPrefix
		}
		printDot()

		slug := generateSlug(post.Title)
		printDot()
		post.MdFilename = prefix + "_" + slug + MarkdownFileExtension
		printDot()

		mgr.ProcessImages(post)
		printDot()

		err = post.GenerateMarkdown()
		if err != nil {
			printXMark("generating md => %s", err)
			errorList = append(errorList, f.Name())
			//fmt.Printf("error while generating markdown: %s => %s\n", post.MdFilename, err)
			continue
		}
		printDot()

		err = mgr.Write(post)
		if err != nil {
			printXMark("writing output => %s", err)
			errorList = append(errorList, f.Name())
			//_, _ = fmt.Fprintf(os.Stderr, "error while writing file: %s => %s\n", post.MdFilename, err)
			continue
		}
		printDot()

		successCount++
		printCheckMark()
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

func newConverterManager(archive string) (*ConverterManager, error) {
	// build the output path value
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

	// create the folders
	// 1. root and output folders
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

	// 2. input folder, unzip will create and extract contents
	files, err := unzipFile(archive, oIn)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("couldn't extract archive: %s => %s", archive, err)
	}

	mediumPosts := filepath.Join(oIn, "posts")
	exists, _ := fileExists(mediumPosts)
	if !exists {
		return nil, fmt.Errorf("couldn't find posts content in the medium extract archive: %s", oIn)
	}

	mgr := &ConverterManager{
		InPath:          oIn,
		MediumPostsPath: mediumPosts,
		OutputPath:      oOut,
		PostsPath:       postsPath,
		ImagesPath:      imagesPath}

	return mgr, nil
}

func newPost(fullPath string) (*Post, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		//_, _ = fmt.Fprintf(os.Stderr, "error while parsing source HTML: %s => %s", f.Name(), err)
		return nil, err
	}

	p := &Post{}
	p.DOM = dom
	p.HTMLFileName = filepath.Base(f.Name())
	p.Images = make([]*Image, 0)
	p.Tags = make([]string, 0)
	p.Lastmod = time.Now().Format(time.RFC3339)

	// draft is prefixed in filename
	p.Draft = strings.HasPrefix(p.HTMLFileName, DraftPrefix)

	return p, nil
}

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

func (mgr *ConverterManager) ReadPosts() ([]os.FileInfo, error) {
	// readFile the files inside medium export posts folder
	files, err := ioutil.ReadDir(mgr.MediumPostsPath)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func (mgr *ConverterManager) DownloadImage(i *Image) error {
	_, err := os.Stat(mgr.ImagesPath)
	if err != nil {
		err = os.MkdirAll(mgr.ImagesPath, os.ModePerm)
		if err != nil {
			//_, _ = fmt.Fprintf(os.Stderr, "couldn't create image dir: %s\n", mgr.ImagesPath)
			return err
		}
	}

	destPath := filepath.Join(mgr.ImagesPath, i.FileName)
	err = downloadFile(i.MediumURL, destPath)
	if err != nil {
		//fmt.Printf("couldn't download image: %s\n", err)
		return err
	}

	return nil
}

func (mgr *ConverterManager) Write(p *Post) error {
	_, err := os.Stat(mgr.PostsPath)
	if err != nil {
		err = os.MkdirAll(mgr.PostsPath, os.ModePerm)
		if err != nil {
			return err
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
		return err
	}

	return nil
}

func (mgr *ConverterManager) ProcessImages(p *Post) {
	images := p.DOM.Find("img")
	printDot()

	if images.Length() == 0 {
		//fmt.Fprintf(os.Stderr, "no images found in the post: %s\n", p.Title)

		return
	}

	//fmt.Fprintf(os.Stderr, "found %d images: %s", images.Length(), p.Title)

	p.Images = make([]*Image, 0)

	images.Each(func(i int, imgDomElement *goquery.Selection) {

		img, err := p.NewImage(imgDomElement, i)
		if err != nil {
			printRedDot()
			//fmt.Fprintf(os.Stderr, "error while reading img element: %s", err)
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

		if _, isFeatured := imgDomElement.Attr("data-is-featured"); isFeatured {
			p.FeaturedImage = img.GetHugoSource()
		}
		printDot()
	})

	//fallback, the featured image is the first one
	if len(p.FeaturedImage) == 0 && len(p.Images) > 0 {
		p.FeaturedImage = p.Images[0].GetHugoSource()
	}
	printDot()

	return
}
