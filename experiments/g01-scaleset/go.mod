module github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset

go 1.27.3

require (
	github.com/1XP-AI/gh-runnerd/experiments/g02-auth v0.0.0
	github.com/actions/scaleset v0.5.0
	github.com/google/uuid v1.6.0
)

replace github.com/1XP-AI/gh-runnerd/experiments/g02-auth => ../g02-auth

require github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
