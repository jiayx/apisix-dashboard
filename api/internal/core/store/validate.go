/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package store

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap/buffer"

	"github.com/apisix/manager-api/conf"
	"github.com/apisix/manager-api/internal/core/entity"
)

type Validator interface {
	Validate(obj interface{}) error
}
type JsonSchemaValidator struct {
	schema *gojsonschema.Schema
}

func NewJsonSchemaValidator(jsonPath string) (Validator, error) {
	bs, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("get abs path failed: %w", err)
	}
	s, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(string(bs)))
	if err != nil {
		return nil, fmt.Errorf("new schema failed: %w", err)
	}
	return &JsonSchemaValidator{
		schema: s,
	}, nil
}

func (v *JsonSchemaValidator) Validate(obj interface{}) error {
	ret, err := v.schema.Validate(gojsonschema.NewGoLoader(obj))
	if err != nil {
		return fmt.Errorf("validate failed: %w", err)
	}

	if !ret.Valid() {
		errString := buffer.Buffer{}
		for i, vErr := range ret.Errors() {
			if i != 0 {
				errString.AppendString("\n")
			}
			errString.AppendString(vErr.String())
		}
		return errors.New(errString.String())
	}
	return nil
}

type APISIXJsonSchemaValidator struct {
	schema *gojsonschema.Schema
}

func NewAPISIXJsonSchemaValidator(jsonPath string) (Validator, error) {
	schemaDef := conf.Schema.Get(jsonPath).String()
	if schemaDef == "" {
		return nil, fmt.Errorf("scheme validate failed: schema not found, path: %s", jsonPath)
	}

	s, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(schemaDef))
	if err != nil {
		return nil, fmt.Errorf("new schema failed: %w", err)
	}
	return &APISIXJsonSchemaValidator{
		schema: s,
	}, nil
}

func getPlugins(reqBody interface{}) map[string]interface{} {
	switch reqBody.(type) {
	case *entity.Route:
		route := reqBody.(*entity.Route)
		return route.Plugins
	case *entity.Service:
		service := reqBody.(*entity.Service)
		return service.Plugins
	case *entity.Consumer:
		consumer := reqBody.(*entity.Consumer)
		return consumer.Plugins
	}
	return nil
}

func cHashKeySchemaCheck(upstream *entity.UpstreamDef) error {
	if upstream.HashOn == "consumer" {
		return nil
	}
	if upstream.HashOn != "vars" &&
		upstream.HashOn != "header" &&
		upstream.HashOn != "cookie" {
		fmt.Errorf("invalid hash_on type: %s", upstream.HashOn)
	}

	var schemaDef string
	if upstream.HashOn == "vars" {
		schemaDef = conf.Schema.Get("main.upstream_hash_vars_schema").String()
		if schemaDef == "" {
			return fmt.Errorf("scheme validate failed: schema not found, patch: main.upstream_hash_vars_schema")
		}
	}

	if upstream.HashOn == "header" || upstream.HashOn == "cookie" {
		schemaDef = conf.Schema.Get("main.upstream_hash_header_schema").String()
		if schemaDef == "" {
			return fmt.Errorf("scheme validate failed: schema not found, path: main.upstream_hash_header_schema")
		}
	}

	s, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(schemaDef))
	if err != nil {
		return fmt.Errorf("scheme validate failed: %w", err)
	}

	ret, err := s.Validate(gojsonschema.NewGoLoader(upstream.Key))
	if err != nil {
		return fmt.Errorf("scheme validate failed: %w", err)
	}

	if !ret.Valid() {
		errString := buffer.Buffer{}
		for i, vErr := range ret.Errors() {
			if i != 0 {
				errString.AppendString("\n")
			}
			errString.AppendString(vErr.String())
		}
		return fmt.Errorf("scheme validate failed: %s", errString.String())
	}

	return nil
}

func checkUpstream(upstream *entity.UpstreamDef) error {
	if upstream == nil {
		return nil
	}

	if upstream.PassHost == "node" && upstream.Nodes != nil {
		if nodes := entity.NodesFormat(upstream.Nodes); len(nodes.([]*entity.Node)) != 1 {
			return fmt.Errorf("only support single node for `node` mode currently")
		}
	}

	if upstream.PassHost == "rewrite" && upstream.UpstreamHost == "" {
		return fmt.Errorf("`upstream_host` can't be empty when `pass_host` is `rewrite`")
	}

	if upstream.Type != "chash" {
		return nil
	}

	//to confirm
	if upstream.HashOn == "" {
		upstream.HashOn = "vars"
	}

	if upstream.HashOn != "consumer" && upstream.Key == "" {
		return fmt.Errorf("missing key")
	}

	if err := cHashKeySchemaCheck(upstream); err != nil {
		return err
	}

	return nil
}

func checkConf(reqBody interface{}) error {
	switch reqBody.(type) {
	case *entity.Route:
		route := reqBody.(*entity.Route)
		if err := checkUpstream(route.Upstream); err != nil {
			return err
		}
	case *entity.Service:
		service := reqBody.(*entity.Service)
		if err := checkUpstream(service.Upstream); err != nil {
			return err
		}
	case *entity.Upstream:
		upstream := reqBody.(*entity.Upstream)
		if err := checkUpstream(&upstream.UpstreamDef); err != nil {
			return err
		}
	}
	return nil
}

func (v *APISIXJsonSchemaValidator) Validate(obj interface{}) error {
	ret, err := v.schema.Validate(gojsonschema.NewGoLoader(obj))
	if err != nil {
		return fmt.Errorf("scheme validate failed: %w", err)
	}

	if !ret.Valid() {
		errString := buffer.Buffer{}
		for i, vErr := range ret.Errors() {
			if i != 0 {
				errString.AppendString("\n")
			}
			errString.AppendString(vErr.String())
		}
		return fmt.Errorf("scheme validate fail: %s", errString.String())
	}

	//custom check
	if err := checkConf(obj); err != nil {
		return err
	}

	//check plugin json schema
	plugins := getPlugins(obj)
	if plugins != nil {
		for pluginName, pluginConf := range plugins {
			schemaDef := conf.Schema.Get("plugins." + pluginName).String()
			if schemaDef == "" {
				return fmt.Errorf("scheme validate failed: schema not found, path: %s", "plugins."+pluginName)
			}

			s, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(schemaDef))
			if err != nil {
				return fmt.Errorf("scheme validate failed: %w", err)
			}

			ret, err := s.Validate(gojsonschema.NewGoLoader(pluginConf))
			if err != nil {
				return fmt.Errorf("scheme validate failed: %w", err)
			}

			if !ret.Valid() {
				errString := buffer.Buffer{}
				for i, vErr := range ret.Errors() {
					if i != 0 {
						errString.AppendString("\n")
					}
					errString.AppendString(vErr.String())
				}
				return fmt.Errorf("scheme validate failed: %s", errString.String())
			}
		}
	}

	return nil
}
