package main

import "fmt"

type Image struct {
	MediumURL, FileName string
}

func (i *Image) GetHugoSource() string {
	return fmt.Sprintf("/%s/%s/%s", HContentType, HImagesDirName, i.FileName)
}
