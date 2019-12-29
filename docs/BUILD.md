Before proceeding with the build, ensure you have
[Go](https://golang.org/doc/install) installed.

Fetch the source code for the latest release, either by:

* cloning the `master` branch:

  ```
  git clone --single-branch --branch master https://github.com/xuoe/kc.git
  ```

* or by downloading the [source code](https://github.com/xuoe/kc/releases/latest):

  ```
  wget -O kc.tar.gz https://github.com/xuoe/kc/archive/master.tar.gz
  tar xvf kc.tar.gz --strip-components 1 --one-top-level
  ```

(If you want to clone or download a specific release, replace `master` with the
desired release in the above `git clone` or `wget` invocation.)

Continue by invoking `make install` with the target installation directory.
Note that, depending on the values of `DESTDIR` and `PREFIX`, this may require
root privileges:

  ```
  cd kc
  make install DESTDIR=<dir> PREFIX=<prefix>
  ```

This builds and installs the binary `kc` and the manual page `kc.1` (gzipped)
into `<dir><prefix>`, such that `find <dir> -name 'kc*'` should print:

  ```
  <dir><prefix>/bin/kc
  <dir><prefix>/share/man/man1/kc.1.gz
  ```

If you do not want `go get` to pull dependencies into your `$GOPATH`, pass
a different value to `make install`.
