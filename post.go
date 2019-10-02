package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/lunny/html2md"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
	MdFilename            string
	HTMLFileName          string
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
		fmt.Fprintf(os.Stderr, "error while reading imgFilename: %s", p.MdFilename)
		return nil, err
	}

	imgFilename := fmt.Sprintf("%s_%d%s", fileNamePrefix, i, ext)
	fmt.Fprintf(os.Stderr, "image name: %s", imgFilename)

	img := &Image{MediumURL: imgSrc, FileName: imgFilename}

	// all successful, attach a reference
	p.Images = append(p.Images, img)

	return img, nil
}

func (p *Post) GetFileNamePrefix() (string, error) {
	if len(strings.TrimSpace(p.MdFilename)) == 0 {
		return "", errors.New("empty filename")
	}

	if filepath.Ext(p.MdFilename) != MarkdownFileExtension {
		return "", errors.New(fmt.Sprintf("invalid filename set: %s", p.MdFilename))
	}

	return strings.TrimSuffix(p.MdFilename, MarkdownFileExtension), nil
}

func (p *Post) SetCanonicalName() {
	canonical := p.DOM.Find(".p-canonical")
	if canonical != nil {
		var exists bool
		//https://coder.today/a-b-tests-developers-manual-f57f5c1a492
		p.FullURL, exists = canonical.Attr("href")
		if exists && len(p.FullURL) > 0 {
			pieces := strings.Split(p.FullURL, "/")
			if len(pieces) > 2 {
				//a-b-tests-developers-manual-f57f5c1a492
				p.Canonical = pieces[len(pieces)-1] //we only need the last part
			}
		}
	}
}

func (p *Post) FixSelfLinks() {
	mediumUsername := "chamilad"
	mediumBaseUrl := fmt.Sprintf("%s/@%s", "https://medium.com", mediumUsername)

	anchors := p.DOM.Find(".markup--anchor")
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

func (p *Post) PopulateTags() error {
	if p.Draft {
		_, _ = fmt.Fprintf(os.Stderr, "not getting tags for draft: %s\n", p.Title)
		return nil
	}

	//TODO make a custom client with a small timeout!
	skipTLS := strings.ToLower(os.Getenv("ALLOW_INSECURE")) == "true"
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
	}
	client := &http.Client{Transport: tr}

	res, err := client.Get(p.FullURL)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	res.Body.Close()
	if err != nil {
		return err
	}

	doc.Find("ul>li>a[href^='/tag']").Each(func(i int, selection *goquery.Selection) {
		p.Tags = append(p.Tags, selection.Text())
	})

	return nil
}

func (p *Post) GenerateMarkdown() error {
	fmt.Printf("Generating markdown %s => %s\n", p.HTMLFileName, p.MdFilename)

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
