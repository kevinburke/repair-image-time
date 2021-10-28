# repair-image-time

A number of photos had incorrect "birth time" or mtimes, that did not reflect
the time the photo was taken or imported. This script tries to find the
information about when the photo was taken, and then repair the file mtime to
reflect that.

I wrote this because Apple sorts photos on your phone based on the file creation
time, so you need to update that to the photo time or else your photos won't
appear in chronological order.
