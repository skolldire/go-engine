{
 sub(/[[:space:]]*\/\/.*$/, "")
 gsub(/^[[:space:]]+|[[:space:]]+$/, "")
 if ($0 == "") next
 if ($1 == "module") {gsub(/"/, "", $2); print "module\t" $2; next}
 if ($1 == "require" || $1 == "replace") {
  section=$1
  if ($2 == "(") {block=section; next}
  offset=2
 } else if ($1 == ")") {block="";next}
 else {section=block;offset=1}
 if (section == "require" && $(offset+1) != "") print "require\t" $offset "\t" $(offset+1)
 if (section == "replace") {
  for(i=offset+1;i<=NF;i++) if($i=="=>") {
   target=$(i+1);gsub(/"/,"",target)
   print "replace\t" $offset "\t" target
   break
  }
 }
}
