webserver-loadtest
==================

My exercise in learning go, this is a way to spin up hits against your
webapp and see when it begins to smoke.  up/down or +/- keys control the
number of concurrent processes loading your url.

This uses the go wrapper around ncurses:goncurses.  That can be a PITA to 
install on anything but the most recent ubuntu, apparently, so for obscure OS's 
like CentOS, Debian, or OS X see 
http://stackoverflow.com/questions/23975235/how-to-build-goncurses-on-os-x-centos-6/.

