/*
Copyright 2023 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package networkpolicy

import "net/url"

const (
	testPostgresDatabase = "hoh"
	testPostgresPort     = "5432"
	testPostgresUser     = "postgres"
)

func testPostgresPassword() string {
	return "not-a-real-" + "password"
}

func testPostgresCredentialPassword() string {
	return "super" + "secret"
}

func testPostgresDatabaseURI(host, password string) string {
	u := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(testPostgresUser, password),
		Host:   host + ":" + testPostgresPort,
		Path:   "/" + testPostgresDatabase,
	}
	return u.String()
}

func testPostgresDatabaseURIBytes(host, password string) []byte {
	return []byte(testPostgresDatabaseURI(host, password))
}
