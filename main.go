// A quick script for converting Medium HTML files to Markdown, suitable for use in a static file generator such as Hugo or Jekyll
//A fork of https://gist.github.com/clipperhouse/010d4666892807afee16ba7711b41401
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/lunny/html2md"

	"github.com/PuerkitoBio/goquery"

"github.com/google/uuid"
)

const (
	HContentType          = "post" // Hugo Content Type, only post is supported
	HImagesDirName        = "images"
	MarkdownFileExtension = ".md"
)

// TODO:
//   1. input medium export zip, not extracted path
//   2. figure out username from medium extract
//   3. flag to create subfolders for posts

type Post struct {
	DOM                   *goquery.Document
	Title, Author, Body   string
	Date, Lastmod         string
	Subtitle, Description string
	Canonical, FullURL    string
	FeaturedImage         string
	Images                []*Image
	Tags                  []string
	Draft                 bool
	Filename              string
	HTMLFileName          string
}

type Image struct {
	MediumURL, FileName string
}

type ConverterManager struct {
	MediumPostsPath, OutputPath, PostsPath, ImagesPath string
}

func (p *Post) NewImage(dom *goquery.Selection, i int) (*Image, error) {
	imgSrc, exists := dom.Attr("src")
	if !exists {
		fmt.Print("warning img no src\n")
		return nil, errors.New("invalid img def, no src found")
	}

	ext := filepath.Ext(imgSrc)
	if len(ext) < 2 {
		ext = ".jpg"
	}

	fileNamePrefix, err := p.GetFileNamePrefix()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while reading imgFilename: %s", p.Filename)
		return nil, err
	}

	imgFilename := fmt.Sprintf("%s_%d%s", fileNamePrefix, i, ext)
	fmt.Fprintf(os.Stderr, "image name: %s", imgFilename)

	//destSrc := fmt.Sprintf("/%s/%s/%s", HContentType, HImagesDirName, imgFilename)

	img := &Image{MediumURL: imgSrc, FileName: imgFilename}

	// the suffix after # is useful for styling the image in a way similar to what medium does
	//imageSrcAttr := fmt.Sprintf("%s#%s", img.Source, extractMediumImageStyle(imgDomElement))
	//imgDomElement.SetAttr("src", imageSrcAttr)
	//fmt.Printf("saved image %s => %s\n", imgSrc, diskPath)

	//img := Image{}
	//img.MediumURL = imgSrc
	//img.FileName = imgFilename

	// all successful, attach a reference
	p.Images = append(p.Images, img)

	//if _, isFeatured := imgDomElement.Attr("data-is-featured"); isFeatured {
	//	p.FeaturedImage = img.Source
	//}

	return img, nil
}

func (i *Image) GetHugoSource() string {
	return fmt.Sprintf("/%s/%s/%s", HContentType, HImagesDirName, i.FileName)
}

func (c *ConverterManager) DownloadImage(i *Image) error {
	_, err := os.Stat(c.ImagesPath)
	if err != nil {
		os.MkdirAll(c.ImagesPath, os.ModePerm)
	}

	destPath := filepath.Join(c.ImagesPath, i.FileName)

	err = DownloadFile(i.MediumURL, destPath)
	if err != nil {
		fmt.Printf("error image: %s\n", err)
		return err
	}

	return nil
}

func (p *Post) GetFileNamePrefix() (string, error) {
	if len(strings.TrimSpace(p.Filename)) == 0 {
		return "", errors.New("empty filename")
	}

	if filepath.Ext(p.Filename) != MarkdownFileExtension {
		return "", errors.New(fmt.Sprintf("invalid filename set: %s", p.Filename))
	}

	return strings.TrimSuffix(p.Filename, MarkdownFileExtension), nil
}

func main() {

	// define input flags
	mPostsPath := flag.String("mediumPosts", "content/posts", "path to medium export posts folder")
	//oPath := flag.String("out", "$(pwd)", "output location")

	flag.Parse()

	// sanitize and validate input
	mPostFPath, err := filepath.Abs(*mPostsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid mediumPosts path: %s", *mPostsPath)
		os.Exit(1)
	}

	_, err = os.Stat(mPostFPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "non-existent mediumPosts path: %s", *mPostsPath)
		os.Exit(1)
	}

	mgr := NewConverterManager(*mPostsPath)

	//if len(strings.TrimSpace(*oPath)) == 0 {

	//// build the output path value
	//pwd, err := os.Getwd()
	//if err != nil {
	//	fmt.Fprintf(os.Stderr, "error while reading cwd: %s", err)
	//	panic(err)
	//}
	//
	//t := time.Now()
	////outFPath := filepath.Join(
	////	pwd,
	////	"m2hout",
	////	fmt.Sprintf("md-%d%02d%02d_%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()))

	err = os.MkdirAll(mgr.OutputPath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	//} else {
	//	outFPath, err := filepath.Abs(*oPath)
	//	if err != nil {
	//		fmt.Fprintf(os.Stderr, "invalid out path: %s", *oPath)
	//		os.Exit(1)
	//	}
	//
	//	_, err = os.Stat(outFPath)
	//	if err != nil {
	//		fmt.Fprintf(os.Stderr, "non-existent out path: %s", *oPath)
	//		os.Exit(1)
	//	}
	//}
	//
	//postsPath := filepath.Join(outFPath, HContentType)
	//imagesPath := filepath.Join(outFPath, HImagesDirName)

	// Destination for Markdown files, perhaps the content folder for Hugo or Jekyll
	//var hugoContentFolder = os.Args[2]
	//if !strings.HasSuffix(hugoContentFolder, "/") {
	//	hugoContentFolder += "/"
	//}

	//var hugoContentType = os.Args[3]

	// readFile the files inside medium export posts folder
	files, err := ioutil.ReadDir(mPostFPath)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Found %d articles.\n", len(files))

	// create images folder
	// TODO: only create if any images are there
	//diskImagesFolder := filepath.Join(outFPath, "images")
	//err = os.MkdirAll(diskImagesFolder, os.ModePerm)
	//if err != nil {
	//	panic(err)
	//}

	// iterate each html file and generate md
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".html") || f.IsDir() {
			//fmt.Printf("Ignoring (ext) %s\n", f.Name())
			continue
		}

		//fpath := filepath.Join(postsHTMLFolder, f.Name())
		fpath := filepath.Join(mPostFPath, f.Name())
		_, err := os.Stat(fpath)
		if err != nil {
			fmt.Println("Error readFile html: ", err)
			continue
		}

		post, err := NewPost(fpath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while reading html file: %s", err)
			continue
		}

		//doc, err := readFile(fpath)
		//if err != nil {
		//	fmt.Println("Error readFile html: ", err)
		//	continue
		//}

		// cleanup unwanted elements
		post.PruneMediumSpecifics()

		// extract interesting values
		post.Lastmod = time.Now().Format(time.RFC3339)
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

		// draft is prefixed in filename
		post.Draft = strings.HasPrefix(f.Name(), "draft_")

		setCanonicalName(post)

		// translate tags for non-draft posts
		if !post.Draft {
			//if we have the URL we can fetch the Tags
			//which are not included in the export html :(
			post.Tags, err = getTagsFor(post.FullURL)
			if err != nil {
				fmt.Printf("error tags: %s\n", err)
				err = nil
			}
		}

		//datetime ISO 2018-09-25T14:13:46.823Z
		//we only keep the date for simplicity
		createdDate := strings.Split(post.Date, "T")[0]
		prefix := createdDate

		if post.Draft {
			prefix = "draft_"
		}

		// hugo/content/article_title/*
		slug := generateSlug(post.Title)
		if len(slug) == 0 {
			slug = "noname_" + post.Date
		}

		//pageBundle := prefix + "_" + slug

		post.Filename = prefix + "_" + slug + MarkdownFileExtension

		mgr.ProcessImages(post)

		//if err != nil {
		//	err = fmt.Errorf("error images folder: %s", err)
		//}

		//fallback, the featured image is the first one
		if len(post.FeaturedImage) == 0 && len(post.Images) > 0 {
			post.FeaturedImage = post.Images[0].GetHugoSource()
		}

		fixSelfLinks(post)

		// ===================================================

		//err = parseDoc(post)
		//
		//if err != nil {
		//	fmt.Println("Error parseDoc: ", err)
		//	//os.RemoveAll(Post.HddFolder)
		//	continue
		//}

		fmt.Printf("Processing %s => %s\n", post.HTMLFileName, post.Filename)

		//outpath := Post.HddFolder + "index.md"

		// convert and write to file
		//outpath := Post.Filename
		//post.Body = docToMarkdown(doc)
		err = post.GenerateMarkdown()
		if err != nil {
			fmt.Printf("Ignoring (empty) %s\n", post.Filename)
			//os.RemoveAll(Post.HddFolder)
			continue
		}

		mgr.Write(post)

		//write(post, filepath.Join(postsPath, post.Filename))
	}
}

func NewConverterManager(m string) *ConverterManager {
	// build the output path value
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while reading cwd: %s", err)
		panic(err)
	}

	t := time.Now()
	o := filepath.Join(
		pwd,
		"m2h_out",
		fmt.Sprintf("md-%d%02d%02d_%02d%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()))

	postsPath := filepath.Join(o, HContentType)
	imagesPath := filepath.Join(postsPath, HImagesDirName)
	mgr := &ConverterManager{MediumPostsPath: m, OutputPath: o, PostsPath: postsPath, ImagesPath: imagesPath}
	return mgr

}

func NewPost(filepath string) (*Post, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while parsing HTML: %s, %s", f.Name(), err)
		return nil, err
	}

	return &Post{DOM: dom, HTMLFileName: f.Name(), Images: make([]*Image, 0)}, nil
}

func fixSelfLinks(post *Post) {
	mediumUsername := "chamilad"
	mediumBaseUrl := fmt.Sprintf("%s/@%s", "https://medium.com", mediumUsername)

	anchors := post.DOM.Find(".markup--anchor")
	if anchors.Length() == 0 {
		fmt.Fprintf(os.Stderr, "no anchors found to replace")
	} else {
		anchors.Each(func(i int, aDomElement *goquery.Selection) {
			original, has := aDomElement.Attr("href")
			if !has {
				return
			}

			if strings.Contains(original, mediumBaseUrl) {
				replaced := strings.TrimPrefix(original, mediumBaseUrl)
				fmt.Fprintf(os.Stderr, "self link found: %s (%s) => %s\n\n\n", original, aDomElement.Text(), replaced)
				aDomElement.SetAttr("href", replaced)
				aDomElement.SetAttr("data-href", replaced)
			}
		})
	}
}

func setCanonicalName(post *Post) {
	canonical := post.DOM.Find(".p-canonical")
	if canonical != nil {
		var exists bool
		//https://coder.today/a-b-tests-developers-manual-f57f5c1a492
		post.FullURL, exists = canonical.Attr("href")
		if exists && len(post.FullURL) > 0 {
			pieces := strings.Split(post.FullURL, "/")
			if len(pieces) > 2 {
				//a-b-tests-developers-manual-f57f5c1a492
				post.Canonical = pieces[len(pieces)-1] //we only need the last part
			}
		}
	}
}

func nbsp(r rune) rune {
	if r == '\u00A0' {
		return ' '
	}

	return r
}

//func parseDoc(post *Post) (err error) {
//	defer func() {
//		if mypanic := recover(); mypanic != nil {
//			err = mypanic.(error)
//		}
//	}()
//
//post.Lastmod = time.Now().Format(time.RFC3339)
//post.Title = doc.Find("title").Text()
//post.Date, _ = doc.Find("time").Attr("datetime")
//post.Author = doc.Find(".p-author.h-card").Text()
//
//subtitle := doc.Find(".p-summary[data-field='subtitle']")
//if subtitle != nil {
//	post.Subtitle = strings.TrimSpace(subtitle.Text())
//}
//desc := doc.Find(".p-summary[data-field='description']")
//if desc != nil {
//	post.Description = strings.TrimSpace(desc.Text())
//}
//
//Medium treats comments/replies as posts
//post.IsComment = doc.Find(".aspectRatioPlaceholder").Length() == 0
// TODO: ^this doesn't work anymore as comments and posts both contain this css class
//
//canonical := doc.Find(".p-canonical")
//if canonical != nil {
//	var exists bool
//	//https://coder.today/a-b-tests-developers-manual-f57f5c1a492
//	post.FullURL, exists = canonical.Attr("href")
//	if exists && len(post.FullURL) > 0 {
//		pieces := strings.Split(post.FullURL, "/")
//		if len(pieces) > 2 {
//			//a-b-tests-developers-manual-f57f5c1a492
//			post.Canonical = pieces[len(pieces)-1] //we only need the last part
//		}
//	}
//}
//
// fix self links
// http://localhost:1313/post/2019-09-19_elasticsearch-on-k8s-01basic-design/
// https://medium.com/@chamilad/elasticsearch-on-k8s-01-basic-design-ecfdaccbb63a
// alias - /elasticsearch-on-k8s-01-basic-design-ecfdaccbb63a
//
// <a
// 		href="https://medium.com/@chamilad/elasticsearch-on-k8s-01-basic-design-ecfdaccbb63a"
// 		data-href="https://medium.com/@chamilad/elasticsearch-on-k8s-01-basic-design-ecfdaccbb63a"
// 		class="markup--anchor markup--li-anchor"
// 		target="_blank">
//
//mediumUsername := "chamilad"
//mediumBaseUrl := fmt.Sprintf("%s/@%s", "https://medium.com", mediumUsername)
//
////TODO: not p-author or p-canonical
//anchors := doc.Find(".markup--anchor")
//if anchors.Length() == 0 {
//	fmt.Fprintf(os.Stderr, "no anchors found to replace")
//} else {
//	anchors.Each(func(i int, aDomElement *goquery.Selection) {
//		original, has := aDomElement.Attr("href")
//		if !has {
//			return
//		}
//
//		if strings.Contains(original, mediumBaseUrl) {
//			replaced := strings.TrimPrefix(original, mediumBaseUrl)
//			fmt.Fprintf(os.Stderr, "self link found: %s (%s) => %s\n\n\n", original, aDomElement.Text(), replaced)
//			aDomElement.SetAttr("href", replaced)
//			aDomElement.SetAttr("data-href", replaced)
//		}
//	})
//}
//
//post.Draft = strings.HasPrefix(f.Name(), "draft_")
//
//if post.Draft == false {
//	//if we have the URL we can fetch the Tags
//	//which are not included in the export html :(
//	post.Tags, err = getTagsFor(post.FullURL)
//	if err != nil {
//		fmt.Printf("error tags: %s\n", err)
//		err = nil
//	}
//}
////datetime ISO 2018-09-25T14:13:46.823Z
////we only keep the date for simplicity
//createdDate := strings.Split(post.Date, "T")[0]
//
//prefix := "draft_"
//if post.Draft == false {
//	prefix = createdDate
//}
//
//// hugo/content/article_title/*
//generateSlug := generateSlug(post.Title)
//if len(generateSlug) == 0 {
//	generateSlug = "noname_" + post.Date
//}
//pageBundle := prefix + "_" + generateSlug
////post.HddFolder = fmt.Sprintf("%s%s/%s/", contentFolder, contentType, pageBundle)
//post.Filename = pageBundle + ".md"
//post.HddFolder = fmt.Sprintf("%s%s/", contentFolder, contentType)
//os.RemoveAll(post.HddFolder) //make sure does not exists
//err = os.MkdirAll(post.HddFolder, os.ModePerm)
//if err != nil {
//	err = fmt.Errorf("error Post folder: %s", err)
//	return
//}
//post.Images, post.FeaturedImage, err = fetchAndReplaceImages(doc, post.HddFolder, contentType, pageBundle)
//post.Images, post.FeaturedImage, err = fetchAndReplaceImages(doc, pageBundle)
//
//if err != nil {
//	err = fmt.Errorf("error images folder: %s", err)
//}
//
////fallback, the featured image is the first one
//if len(post.FeaturedImage) == 0 && len(post.Images) > 0 {
//	post.FeaturedImage = post.Images[0]
//}
//
//return
//}

func (p *Post) GenerateMarkdown() error {
	body := ""
	p.DOM.Find("div.section-inner").Each(func(i int, s *goquery.Selection) {
		h, _ := s.Html()
		body += html2md.Convert(strings.TrimSpace(h))
	})

	body = strings.Map(func(r rune) rune {
		if r == '\u00A0' {
			return ' '
		}

		return r
	}, body)

	p.Body = strings.TrimSpace(body)

	if len(p.Body) == 0 {
		return errors.New("empty markdown generated")
	}

	return nil
}

// PruneMediumSpecifics removes unwanted elements in the HTML document
func (p *Post) PruneMediumSpecifics() {
	//fix the big previews boxes for URLS, we don't need the description and other stuff
	//html2md cannot handle them (is a div of <a> that contains <strong><br><em>...)
	p.DOM.Find(".graf .markup--mixtapeEmbed-anchor").Each(func(i int, link *goquery.Selection) {
		title := link.Find("strong")
		if title == nil {
			return
		}

		//remove the <strong> <br> and <em>
		link.Empty()

		link.SetText(strings.TrimSpace(title.Text()))
	})

	//remove the empty URLs (which is the thumbnail on medium)
	p.DOM.Find(".graf a.mixtapeImage").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})

	//remove the TITLE, it is already as metadata in the markdown
	p.DOM.Find("h3.graf--title").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})

	p.DOM.Find("h1").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})
}

//func readFile(path string) (*Post, error) {
//	f, err := os.Open(path)
//	if err != nil {
//		panic(err)
//	}
//	defer f.Close()
//
//	// Load the HTML document
//	dom, err := goquery.NewDocumentFromReader(f)
//	if err != nil {
//		fmt.Fprintf(os.Stderr, "error while parsing HTML: %s, %s", f.Name(), err)
//		return nil, err
//	}
//
//	post := Post{DOM: dom}
//	return &post, nil
//}

//func write(post Post, path string) {
//	//os.Remove(path)
//	f, err := os.Create(path)
//	if err != nil {
//		panic(err)
//	}
//	defer f.Close()
//
//	err = tmpl.Execute(f, post)
//	if err != nil {
//		panic(err)
//	}
//}

func (c *ConverterManager) Write(p *Post) {
	_, err := os.Stat(c.PostsPath)
	if err != nil {
		os.MkdirAll(c.PostsPath, os.ModePerm)
	}

	f, err := os.Create(filepath.Join(c.PostsPath, p.Filename))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	err = tmpl.Execute(f, p)
	if err != nil {
		panic(err)
	}
}

var spaces = regexp.MustCompile(`[\s]+`)
var notallowed = regexp.MustCompile(`[^\p{L}\p{N}.\s]`)
var athe = regexp.MustCompile(`^(a\-|the\-)`)

func generateSlug(s string) string {
	result := s
	result = strings.Replace(result, "%", " percent", -1)
	result = strings.Replace(result, "#", " sharp", -1)
	result = notallowed.ReplaceAllString(result, "")
	result = spaces.ReplaceAllString(result, "-")
	result = strings.ToLower(result)
	result = athe.ReplaceAllString(result, "")

	return result
}

var tmpl = template.Must(template.New("").Parse(`---
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
`))

func getTagsFor(url string) ([]string, error) {
	//TODO make a custom client with a small timeout!
	skipTLS := strings.ToLower(os.Getenv("ALLOW_INSECURE")) == "true"
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
	}
	client := &http.Client{Transport: tr}

	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	var result []string
	//fmt.Printf("%s", doc.Text())
	doc.Find("ul>li>a[href^='/tag']").Each(func(i int, selection *goquery.Selection) {
		result = append(result, selection.Text())
	})
	return result, nil
}

func (c *ConverterManager) ProcessImages(p *Post) {
	images := p.DOM.Find("img")
	if images.Length() == 0 {
		fmt.Fprintf(os.Stderr, "no images found in the post: %s\n", p.Title)
		return
	}

	fmt.Fprintf(os.Stderr, "found %d images: %s", images.Length(), p.Title)

	//diskImagesFolder := folder + "images/"
	//diskImagesFolder := filepath.Join(outFPath, "images")
	//err := os.Mkdir(diskImagesFolder, os.ModePerm)
	//if err != nil {
	//	return nil, "", fmt.Errorf("error images folder: %s\n", err)
	//}

	//var index int
	//var featuredImage string
	//var result []Image
	p.Images = make([]*Image, 0)

	images.Each(func(i int, imgDomElement *goquery.Selection) {
		//index++

		img, err := p.NewImage(imgDomElement, i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error while reading img element: %s", err)
			return
		}

		//imgSrc, exists := imgDomElement.Attr("src")
		//if !exists {
		//	fmt.Print("warning img no src\n")
		//	return
		//}
		//
		//img.MediumURL = imgSrc
		//
		//ext := filepath.Ext(img.MediumURL)
		//if len(ext) < 2 {
		//	ext = ".jpg"
		//}
		//
		//fileNamePrefix, err := p.GetFileNamePrefix()
		//if err != nil {
		//	fmt.Fprintf(os.Stderr, "error while reading imgFilename: %s", p.Filename)
		//	return
		//}
		//
		//imgFilename := fmt.Sprintf("%s_%d%s", fileNamePrefix, index, ext)
		//fmt.Fprintf(os.Stderr, "image name: %s", imgFilename)
		//
		//img.FileName = imgFilename

		//diskPath := fmt.Sprintf("%s%s", diskImagesFolder, imgFilename)
		//diskPath := filepath.Join(imagesPath, imgFilename)
		//fmt.Fprintf(os.Stderr, "image disk path: %s", diskPath)

		c.DownloadImage(img)

		// TODO: download later
		//err = DownloadFile(imgSrc, diskPath)
		//if err != nil {
		//	fmt.Printf("error image: %s\n", err)
		//	return
		//}

		//we presume that folder is the hugo/static/img folder
		//url := fmt.Sprintf("/%s/%s/images/%s", contentType, pageBundle, imgFilename)
		//url := fmt.Sprintf("/%s/images/%s", HContentType, imgFilename)

		//sourceUrl, err := generateURLFromFileName(img.FileName)
		//if err != nil {
		//	fmt.Fprintf(os.Stderr, "error while generating img src url: %s", img.FileName)
		//	return
		//}
		//
		//img.Source = sourceUrl

		// the suffix after # is useful for styling the image in a way similar to what medium does
		imageSrcAttr := fmt.Sprintf("%s#%s", img.GetHugoSource(), extractMediumImageStyle(imgDomElement))
		imgDomElement.SetAttr("src", imageSrcAttr)
		//fmt.Printf("saved image %s => %s\n", imgSrc, diskPath)

		//img := Image{}
		//img.MediumURL = imgSrc
		//img.FileName = imgFilename

		//p.Images = append(p.Images, img)

		if _, isFeatured := imgDomElement.Attr("data-is-featured"); isFeatured {
			p.FeaturedImage = img.GetHugoSource()
		}
	})

	return

	//return result, featuredImage, nil
}

//func generateURLFromFileName(filename string) (string, error) {
//	return fmt.Sprintf("/%s/%s/%s", HContentType, HImagesDirName, filename), nil
//}

var mediumImageLayout = regexp.MustCompile(`graf--(layout\w+)`)

func extractMediumImageStyle(imgDomElement *goquery.Selection) (mediumImageStyle string) {
	figure := imgDomElement.ParentsUntil("figure.graf").Parent()
	imageStyles := figure.AttrOr("class", "")
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

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func DownloadFile(url, filepath string) error {
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
