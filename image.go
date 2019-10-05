package main

import "fmt"

// Image represents details of an img element in an HTML document
type Image struct {
	MediumURL, FileName string
}

// GetHugoSource returns the value to be used for a given image. This value
// points to the downloaded location relative to a Hugo source root
func (i *Image) GetHugoSource() string {
	return fmt.Sprintf("/%s/%s/%s", HContentType, HImagesDirName, i.FileName)
}
