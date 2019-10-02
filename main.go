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
//   1. input medium export zip, not extracted path
//   2. figure out username from medium extract - profile/profile.html .u-url
//   3. flag to create subfolders for posts

type ConverterManager struct {
	MediumPostsPath, OutputPath, PostsPath, ImagesPath string
}

func main() {
	// define input flags
	zipF := flag.String("f", "medium-export.zip", "the medium-export.zip file from Medium")
	flag.Parse()

	// sanitize and validate input
	exists, zipFilePath := fileExists(zipF)
	if !exists {
		printError("couldn't read give medium extract archive: %s\n", zipFilePath)
		os.Exit(1)
	}

	mgr := newConverterManager(zipFilePath)
	mgr.CreateOutputDirs()
	files := mgr.ReadPosts()

	color.Green("Found %d articles.", len(files))
	//fmt.Printf("Found %d articles.\n", len(files))

	// iterate each html file and generate md
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".html") || f.IsDir() {
			//fmt.Printf("Ignoring (ext) %s\n", f.Name())
			continue
		}

		fpath := filepath.Join(mgr.MediumPostsPath, f.Name())
		_, err := os.Stat(fpath)
		if err != nil {
			printError("couldn't read html file: %s => %s", f.Name(), err)
			continue
		}

		post, err := newPost(fpath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error while reading html file: %s => %s\n", f.Name(), err)
			continue
		}

		// cleanup unwanted elements
		post.PruneMediumSpecifics()

		// query dom for interesting values
		post.Date, _ = post.DOM.Find("time").Attr("datetime")
		post.Author = post.DOM.Find(".p-author.h-card").Text()
		post.Title = strings.TrimSpace(post.DOM.Find("title").Text())
		// if title is empty, name it with a random string
		if len(post.Title) == 0 {
			post.Title = fmt.Sprintf("untitled_%s", uuid.New().String())
		}

		subtitle := post.DOM.Find(".p-summary[data-field='subtitle']")
		if subtitle != nil {
			post.Subtitle = strings.TrimSpace(subtitle.Text())
		}

		desc := post.DOM.Find(".p-summary[data-field='description']")
		if desc != nil {
			post.Description = strings.TrimSpace(desc.Text())
		}

		post.SetCanonicalName()
		post.FixSelfLinks()

		err = post.PopulateTags()
		if err != nil {
			fmt.Printf("error while collecting tags: %s\n", err)
		}

		//datetime ISO 2018-09-25T14:13:46.823Z
		//we only keep the date for simplicity
		createdDate := strings.Split(post.Date, "T")[0]
		prefix := createdDate

		if post.Draft {
			prefix = DraftPrefix
		}

		slug := generateSlug(post.Title)
		post.MdFilename = prefix + "_" + slug + MarkdownFileExtension

		mgr.ProcessImages(post)

		err = post.GenerateMarkdown()
		if err != nil {
			fmt.Printf("error while generating markdown: %s => %s\n", post.MdFilename, err)
			continue
		}

		err = mgr.Write(post)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error while writing file: %s => %s\n", post.MdFilename, err)
			continue
		}
	}
}

func newConverterManager(archive string) *ConverterManager {
	// build the output path value
	pwd, err := os.Getwd()
	if err != nil {
		printError("error while reading cwd: %s\n", err)
		panic(err)
	}

	t := time.Now()
	tmpPath := os.TempDir()
	tmpIn := filepath.Join(tmpPath,
		fmt.Sprintf("m2h_tmp-%d%02d%02d_%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()))

	files, err := unzipFile(archive, tmpIn)
	if err != nil || len(files) == 0 {
		printError("invalid archive: %s\n", archive)
	}

	mediumPosts := filepath.Join(tmpIn, "posts")

	exists, _ := fileExists(&mediumPosts)
	if !exists {
		printError("couldn't find posts content in the medium extract archive: %s", tmpIn)
		return nil
	}

	o := filepath.Join(
		pwd,
		"m2h_out",
		fmt.Sprintf("md-%d%02d%02d_%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()))

	postsPath := filepath.Join(o, HContentType)
	imagesPath := filepath.Join(postsPath, HImagesDirName)
	mgr := &ConverterManager{MediumPostsPath: mediumPosts, OutputPath: o, PostsPath: postsPath, ImagesPath: imagesPath}
	return mgr

}

func newPost(filepath string) (*Post, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error while parsing source HTML: %s => %s", f.Name(), err)
		return nil, err
	}

	p := &Post{}
	p.DOM = dom
	p.HTMLFileName = f.Name()
	p.Images = make([]*Image, 0)
	p.Tags = make([]string, 0)
	p.Lastmod = time.Now().Format(time.RFC3339)

	// draft is prefixed in filename
	p.Draft = strings.HasPrefix(p.HTMLFileName, DraftPrefix)

	return p, nil
}

func (mgr *ConverterManager) ReadPosts() []os.FileInfo {
	// readFile the files inside medium export posts folder
	files, err := ioutil.ReadDir(mgr.MediumPostsPath)
	if err != nil {
		panic(err)
	}

	return files
}

func (mgr *ConverterManager) CreateOutputDirs() {
	err := os.MkdirAll(mgr.OutputPath, os.ModePerm)
	if err != nil {
		panic(err)
	}
}

func (mgr *ConverterManager) DownloadImage(i *Image) error {
	_, err := os.Stat(mgr.ImagesPath)
	if err != nil {
		err = os.MkdirAll(mgr.ImagesPath, os.ModePerm)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "couldn't create image dir: %s\n", mgr.ImagesPath)
			return err
		}
	}

	destPath := filepath.Join(mgr.ImagesPath, i.FileName)
	err = downloadFile(i.MediumURL, destPath)
	if err != nil {
		fmt.Printf("couldn't download image: %s\n", err)
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
	if images.Length() == 0 {
		fmt.Fprintf(os.Stderr, "no images found in the post: %s\n", p.Title)
		return
	}

	fmt.Fprintf(os.Stderr, "found %d images: %s", images.Length(), p.Title)

	p.Images = make([]*Image, 0)

	images.Each(func(i int, imgDomElement *goquery.Selection) {

		img, err := p.NewImage(imgDomElement, i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while reading img element: %s", err)
			return
		}

		mgr.DownloadImage(img)

		// the suffix after # is useful for styling the image in a way similar to what medium does
		imageSrcAttr := fmt.Sprintf("%s#%s", img.GetHugoSource(), extractMediumImageStyle(imgDomElement))
		imgDomElement.SetAttr("src", imageSrcAttr)

		if _, isFeatured := imgDomElement.Attr("data-is-featured"); isFeatured {
			p.FeaturedImage = img.GetHugoSource()
		}
	})

	//fallback, the featured image is the first one
	if len(p.FeaturedImage) == 0 && len(p.Images) > 0 {
		p.FeaturedImage = p.Images[0].GetHugoSource()
	}

	return
}
