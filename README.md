# Go Web Auth Checker

The Go Web Auth Checker (gowac) security tool (re-write of wac in Go) will check a supplied list of urls using auth options supplied to see if access is granted or not, 
this can be used to expose security holes where certain urls have not been locked down.

You would typically compile a list of urls into a file either by using a tool to spider the site or by building it via the directory
structure.

**Only `GET` requests are supported.**

## Options

```
Usage:
  gowac [OPTIONS] URLs

Application Options:
  -v, --verbose   Show verbose debug information
  -t, --threads=  Number of request threads (default: 10)
  -c, --cookie=
  -a, --auth=     Authorization to use for requests in format username:password
  -w, --wait=     Number of seconds to wait before timing out request (default: 5)
  -s, --status=   Check for specific status code returned such as 401
  -r, --redirect= Check for redirect of 301/302 and Location header
  -b, --body=     Check for custom body content returned such as 'login is invalid'

Help Options:
  -h, --help      Show this help message
```

## Examples

```
gowac -r '/auth/login' site_urls.txt # anonymous test redirect

gowac -c 'MY_COOKIE_STRING' -r '/auth/login' site_urls.txt # cookie test redirect

gowac -a user:password -s 401 site_urls.txt # basic auth test 401 response

gowac -c 'MY_COOKIE_STRING' -b 'access denied' site_urls.txt # cookie test body has string inside
```
