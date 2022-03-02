// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mod_cors

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
	"github.com/bfenetworks/bfe/bfe_util/json"
)

type CorsRuleFile struct {
	Version string
	Config  ProductRuleRawList // product -> raw rule list
}

type CorsRuleConf struct {
	Version string
	Config  ProductRuleList // product -> rule list
}

type CorsRuleRaw struct {
	Cond string // condition

	// AccessControlAllowOrigins specifies either a single origin, which tells browsers to
	// allow that origin to access the resource; or else — for requests without credentials —
	// the "*" wildcard, to tell browsers to allow any origin to access the resource
	AccessControlAllowOrigins []string

	// AccessControlAllowCredentials Indicates whether or not the response to the request can be exposed
	AccessControlAllowCredentials bool

	// AccessControlExposeHeaders lets a server whitelist headers that browsers are allowed to access.
	AccessControlExposeHeaders []string

	// AccessControlAllowMethods specifies the method or methods allowed when accessing the resource.
	// This is used in response to a preflight request.
	AccessControlAllowMethods []string

	// AccessControlAllowHeaders indicates which HTTP headers can be used when making the actual request.
	// This is used in response to a preflight request.
	AccessControlAllowHeaders []string

	// AccessControlMaxAge indicates how long the results of a preflight request can be cached.
	// This is used in response to a preflight request.
	AccessControlMaxAge *int
}

type ProductRuleRawList map[string]RuleRawList // product => raw rule list
type RuleRawList []CorsRuleRaw

var (
	supportedMethod = map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodDelete:  true,
		http.MethodConnect: true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
		http.MethodPatch:   true,
	}
)

func CorsRuleCheck(corsRuleFile *CorsRuleFile) error {
	if corsRuleFile == nil {
		return fmt.Errorf("corsRuleFile is nil")
	}

	if len(corsRuleFile.Version) == 0 {
		return fmt.Errorf("no Version")
	}

	if corsRuleFile.Config == nil {
		return fmt.Errorf("no Config")
	}

	return nil
}

func ruleConvert(rawRule CorsRuleRaw) (*CorsRule, error) {
	cond, err := condition.Build(rawRule.Cond)
	if err != nil {
		return nil, err
	}

	var rule CorsRule
	rule.Cond = cond

	if len(rawRule.AccessControlAllowOrigins) == 0 {
		return nil, fmt.Errorf("AccessControlAllowOrigins not set")
	}

	rule.AccessControlAllowOriginMap = make(map[string]bool)

	// <origin>: 	 Specifies the list of supported origin
	// * (wildcard): For requests without credentials, the literal value "*" can be specified, as a wildcard;
	// 	  			 the value tells browsers to allow requesting code from any origin to access the resource.
	//    			 Attempting to use the wildcard with credentials will result in an error.
	// null: 		 Specifies the origin "null".
	// %origin: 	 Specifies the origin from the request header "Origin"
	for _, allowOrigin := range rawRule.AccessControlAllowOrigins {
		if strings.HasPrefix(allowOrigin, "%") && allowOrigin != "%origin" {
			return nil, fmt.Errorf("AccessControlAllowOrigins %s is not supported", allowOrigin)
		}

		if strings.Contains(allowOrigin, "*") && len(allowOrigin) != 1 {
			return nil, fmt.Errorf("AccessControlAllowOrigins %s is not supported", allowOrigin)
		}

		if allowOrigin == "*" && rawRule.AccessControlAllowCredentials {
			return nil, fmt.Errorf("AccessControlAllowCredentials can not be true when AccessControlAllowOrigins is *")
		}

		if (allowOrigin == "null" || allowOrigin == "*") && len(rawRule.AccessControlAllowOrigins) != 1 {
			return nil, fmt.Errorf("AccessControlAllowOrigins can only contain one element when AccessControlAllowOrigins is null or *")
		}

		rule.AccessControlAllowOriginMap[allowOrigin] = true
	}

	// <header-name>: The name of a supported request header. The header may list any number of headers
	// * (wildcard):  The value "*" only counts as a special wildcard value for requests without credentials
	// 				  (requests without HTTP cookies or HTTP authentication information).
	// 				  In requests with credentials, it is treated as the literal header name "*" without special semantics.
	// 				  Note that the Authorization header can't be wildcarded and always needs to be listed explicitly.
	for _, allowHeader := range rawRule.AccessControlAllowHeaders {
		if strings.Contains(allowHeader, "*") && len(allowHeader) != 1 {
			return nil, fmt.Errorf("AccessControlAllowHeaders %s is not supported", allowHeader)
		}

		if allowHeader == "*" && len(rawRule.AccessControlAllowHeaders) != 1 {
			return nil, fmt.Errorf("AccessControlAllowHeaders can only contain one element when AccessControlAllowHeaders is *")
		}
	}
	rule.AccessControlAllowHeaders = rawRule.AccessControlAllowHeaders

	// <header-name>: A list of exposed headers consisting of zero or more header names other than the CORS-safelisted request headers
	// 				  that the resource might use and can be exposed.
	// * (wildcard):  The value "*" only counts as a special wildcard value for requests without credentials
	// 				  (requests without HTTP cookies or HTTP authentication information).
	// 				  In requests with credentials, it is treated as the literal header name "*" without special semantics.
	// 			 	  Note that the Authorization header can't be wildcarded and always needs to be listed explicitly.
	for _, exposeHeader := range rawRule.AccessControlExposeHeaders {
		if strings.Contains(exposeHeader, "*") && len(exposeHeader) != 1 {
			return nil, fmt.Errorf("AccessControlExposeHeaders %s is not supported", exposeHeader)
		}

		if exposeHeader == "*" && len(rawRule.AccessControlExposeHeaders) != 1 {
			return nil, fmt.Errorf("AccessControlExposeHeaders can only contain one element when AccessControlExposeHeaders is *")
		}
	}
	rule.AccessControlExposeHeaders = rawRule.AccessControlExposeHeaders

	// <method>: 	 list of the allowed HTTP request methods.
	// * (wildcard): The value "*" only counts as a special wildcard value for requests without
	// 				 credentials (requests without HTTP cookies or HTTP authentication information).
	// 				 In requests with credentials, it is treated as the literal method name "*" without special semantics.
	for _, allowMethod := range rawRule.AccessControlAllowMethods {
		if strings.Contains(allowMethod, "*") && len(allowMethod) != 1 {
			return nil, fmt.Errorf("AccessControlAllowMethods %s is not supported", allowMethod)
		}

		if allowMethod == "*" && len(rawRule.AccessControlAllowMethods) != 1 {
			return nil, fmt.Errorf("AccessControlAllowMethods can only contain one element when AccessControlAllowMethods is *")
		}

		if allowMethod != "*" {
			if _, ok := supportedMethod[allowMethod]; !ok {
				return nil, fmt.Errorf("AccessControlAllowMethods %s is not supported", allowMethod)
			}
		}
	}
	rule.AccessControlAllowMethods = rawRule.AccessControlAllowMethods

	// Maximum number of seconds the results can be cached.
	// Firefox caps this at 24 hours (86400 seconds).
	// Chromium (prior to v76) caps at 10 minutes (600 seconds).
	// Chromium (starting in v76) caps at 2 hours (7200 seconds).
	// Chromium also specifies a default value of 5 seconds.
	// A value of -1 will disable caching, requiring a preflight OPTIONS check for all calls.
	if rawRule.AccessControlMaxAge != nil {
		if *rawRule.AccessControlMaxAge < -1 || *rawRule.AccessControlMaxAge > 86400 {
			return nil, fmt.Errorf("AccessControlMaxAge must be in [-1, 86400]")
		}
		rule.AccessControlMaxAge = rawRule.AccessControlMaxAge
	}

	rule.AccessControlAllowCredentials = rawRule.AccessControlAllowCredentials
	return &rule, nil
}

func ruleListConvert(rawRuleList RuleRawList) (CorsRuleList, error) {
	ruleList := CorsRuleList{}
	for i, rawRule := range rawRuleList {
		rule, err := ruleConvert(rawRule)
		if err != nil {
			return nil, fmt.Errorf("rule [%d] error: %v", i, err)
		}

		ruleList = append(ruleList, *rule)
	}

	return ruleList, nil
}

func CorsRuleFileLoad(filename string) (*CorsRuleConf, error) {
	var corsRuleFile CorsRuleFile
	var corsRuleConf CorsRuleConf

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&corsRuleFile)
	if err != nil {
		return nil, err
	}

	err = CorsRuleCheck(&corsRuleFile)
	if err != nil {
		return nil, err
	}

	corsRuleConf.Version = corsRuleFile.Version
	corsRuleConf.Config = make(ProductRuleList)

	for product, ruleFileList := range corsRuleFile.Config {
		ruleList, err := ruleListConvert(ruleFileList)
		if err != nil {
			return nil, fmt.Errorf("product[%s] rule error: %v", product, err)
		}
		corsRuleConf.Config[product] = ruleList
	}

	return &corsRuleConf, nil
}
