// Package v1alpha1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.3.0 DO NOT EDIT.
package v1alpha1

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strings"

	externalRef0 "github.com/flightctl/flightctl/api/v1alpha1"
	"github.com/getkin/kin-openapi/openapi3"
)

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	res := make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	pathPrefix := path.Dir(pathToFile)

	for rawPath, rawFunc := range externalRef0.PathToRawSpec(path.Join(pathPrefix, "../openapi.yaml")) {
		if _, ok := res[rawPath]; ok {
			// it is not possible to compare functions in golang, so always overwrite the old value
		}
		res[rawPath] = rawFunc
	}
	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	resolvePath := PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		pathToFile := url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
