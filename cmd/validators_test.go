package cmd

import (
	"testing"

	"github.com/Azure/azure-storage-azcopy/v10/common"
	"github.com/stretchr/testify/assert"
)

func TestValidateArgumentLocation(t *testing.T) {
	a := assert.New(t)

	test := []struct {
		src                   string
		userSpecifiedLocation string

		expectedLocation common.Location
		expectedError    string
	}{
		// User does not specify location
		{"https://test.blob.core.windows.net/container1", "", common.ELocation.Blob(), ""},
		{"https://test.file.core.windows.net/container1", "", common.ELocation.File(), ""},
		{"https://test.dfs.core.windows.net/container1", "", common.ELocation.BlobFS(), ""},
		{"https://s3.amazonaws.com/bucket", "", common.ELocation.S3(), ""},
		{"https://storage.cloud.google.com/bucket", "", common.ELocation.GCP(), ""},
		// Generic HTTP URLs should be detected as Http
		{"https://privateendpoint.com/container1", "", common.ELocation.Http(), ""},
		{"http://127.0.0.1:10000/devstoreaccount1/container1", "", common.ELocation.Http(), ""},
		{"https://cdn.example.com/public/file.bin", "", common.ELocation.Http(), ""},

		// User specifies location
		{"https://privateendpoint.com/container1", "FILE", common.ELocation.File(), ""},
		{"http://127.0.0.1:10000/devstoreaccount1/container1", "BloB", common.ELocation.Blob(), ""},
		{"https://test.file.core.windows.net/container1", "blobfs", common.ELocation.BlobFS(), ""}, // Tests that the endpoint does not really matter
		{"https://privateendpoint.com/container1", "random", common.ELocation.Unknown(), "invalid --location value specified"},
	}

	for _, v := range test {
		loc, err := ValidateArgumentLocation(v.src, v.userSpecifiedLocation)
		a.Equal(v.expectedLocation, loc)
		a.Equal(err == nil, v.expectedError == "")
		if err != nil {
			a.Contains(err.Error(), v.expectedError)
		}
	}
}

func TestInferArgumentLocation(t *testing.T) {
	a := assert.New(t)

	test := []struct {
		src              string
		expectedLocation common.Location
	}{
		{"https://test.blob.core.windows.net/container8", common.ELocation.Blob()},
		{"https://test.file.core.windows.net/container23", common.ELocation.File()},
		{"https://test.dfs.core.windows.net/container45", common.ELocation.BlobFS()},
		{"https://s3.amazonaws.com/bucket", common.ELocation.S3()},
		{"https://storage.cloud.google.com/bucket", common.ELocation.GCP()},
		// Generic HTTP URLs should now be detected as Http location
		{"https://api.example.com/files/data.bin", common.ELocation.Http()},
		{"http://download.example.com/archive.tar.gz", common.ELocation.Http()},
		{"https://cdn.mysite.com/videos/video.mp4", common.ELocation.Http()},
		{"http://localhost:8080/file.txt", common.ELocation.Http()},
		// IP addresses with HTTP should be Http (not Unknown)
		{"http://192.168.1.1:8000/file.dat", common.ELocation.Http()},
		{"http://127.0.0.1:10000/devstoreaccount1/container1", common.ELocation.Http()},
	}

	for _, v := range test {
		loc := InferArgumentLocation(v.src)
		a.Equal(v.expectedLocation, loc)
  }
}
