#!/usr/bin/env python

import sys 

sys.stderr = sys.stdout 

import os 
from cgi import escape 

print "Content-type: text/html" 
print 
print "<html><head><title>Situation snapshot</title></head><body><p>"
print "Running:"
print "<b>Python %s</b><br><br>" %(sys.version)
print "Environmental variables:<br>"
print "<ul>"
for k in sorted(os.environ):
    print "<li><b>%s:</b>\t\t%s<br>" %(escape(k), escape(os.environ[k]))

print "</ul></p></body></html>" 
