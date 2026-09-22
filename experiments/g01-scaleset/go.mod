module github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset

go 1.26.8

require (
	github.com/1XP-AI/gh-runnerd/experiments/g02-auth v0.0.0
	github.com/actions/scaleset v0.4.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-retryablehttp v0.7.8
)

replace github.com/1XP-AI/gh-runnerd/experiments/g02-auth => ../g02-auth

require (
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
)
