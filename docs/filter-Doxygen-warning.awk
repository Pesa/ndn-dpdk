#!/usr/bin/awk -f
/warning: The following parameter[s]? of .* (is|are) not documented:/ {
  skip = 1
  next
}
$0 ~ /^  parameter/ {
  if (!skip) {
    print
  }
}
$0 !~ /^  / {
  skip = 0
  print
}
