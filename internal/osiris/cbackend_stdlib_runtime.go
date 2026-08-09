package osiris

// cRuntimeStdlibComponent contains the bounded, host-independent part of the
// standard library.  It is deliberately a separate component: the native
// emitter owns symbol selection while this file owns C-side allocation,
// validation, and value construction.  Every result is either copied into a
// tracked allocation or is a value whose lifetime is owned by an already
// tracked input.
const cRuntimeStdlibComponent = `
static int ostd_i64(OValue value, int64_t *out) {
  double n;
  if(!onum(value,&n)||!isfinite(n)||trunc(n)!=n||n<(-9223372036854775807.0-1.0)||n>9223372036854775807.0) return 0;
  *out=(int64_t)n; return 1;
}
static int ostd_math_unsigned(OValue value, uint64_t *out, uint16_t *width) {
  if(value.kind!=O_UINT || value.fbits==0 || value.fbits>64) return 0;
  *out=value.as.u; *width=value.fbits; return 1;
}
static OValue ostd_math_gcd(OValue left, OValue right) {
  uint64_t a,b; uint16_t left_width,right_width;
  if(!ostd_math_unsigned(left,&a,&left_width)||!ostd_math_unsigned(right,&b,&right_width)) return ofail("gcd expects fixed-width unsigned integers");
  while(b!=0) { uint64_t remainder=a%b; a=b; b=remainder; }
  return ouintbits(a,left_width>right_width?left_width:right_width);
}
static OValue ostd_math_lcm(OValue left, OValue right) {
  uint64_t a,b; uint16_t left_width,right_width;
  if(!ostd_math_unsigned(left,&a,&left_width)||!ostd_math_unsigned(right,&b,&right_width)) return ofail("lcm expects fixed-width unsigned integers");
  uint16_t width=left_width>right_width?left_width:right_width;
  if(a==0||b==0) return ouintbits(0,width);
  uint64_t x=a,y=b;
  while(y!=0) { uint64_t remainder=x%y; x=y; y=remainder; }
  uint64_t reduced=a/x;
  if(reduced>UINT64_MAX/b || reduced*b>omask(width)) return ofail("lcm result exceeds its unsigned width");
  return ouintbits(reduced*b,width);
}
static OValue ostd_math_divmod(OValue dividend, OValue divisor) {
  uint64_t left,right; uint16_t left_width,right_width;
  if(!ostd_math_unsigned(dividend,&left,&left_width)||!ostd_math_unsigned(divisor,&right,&right_width)) return ofail("divmod expects fixed-width unsigned integers");
  if(right==0) return ofail("divmod divisor must be non-zero");
  uint16_t width=left_width>right_width?left_width:right_width;
  OValue parts[2]={ouintbits(left/right,width),ouintbits(left%right,width)};
  return olist(parts,2);
}
static int ostd_math_bit_input(OValue value, uint64_t *out, uint16_t *width) {
  if(!ostd_math_unsigned(value,out,width)) return 0;
  *out &= omask(*width);
  return 1;
}
static OValue ostd_math_count_ones(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("count_ones expects a fixed-width unsigned integer");
  uint64_t count=0;
  for(uint16_t bit=0; bit<width; bit++) count += (raw>>bit)&UINT64_C(1);
  return ouintbits(count,64);
}
static OValue ostd_math_count_zeros(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("count_zeros expects a fixed-width unsigned integer");
  uint64_t count=0;
  for(uint16_t bit=0; bit<width; bit++) count += ((raw>>bit)&UINT64_C(1))==0;
  return ouintbits(count,64);
}
static OValue ostd_math_leading_zeros(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("leading_zeros expects a fixed-width unsigned integer");
  uint16_t count=0;
  for(uint16_t bit=width; bit>0; bit--) { if((raw>>(bit-1))&UINT64_C(1)) break; count++; }
  return ouintbits(count,64);
}
static OValue ostd_math_trailing_zeros(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("trailing_zeros expects a fixed-width unsigned integer");
  uint16_t count=0;
  while(count<width && ((raw>>count)&UINT64_C(1))==0) count++;
  return ouintbits(count,64);
}
static OValue ostd_math_bit_width(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("bit_width expects a fixed-width unsigned integer");
  uint16_t result=0;
  while(raw!=0) { raw>>=1; result++; }
  return ouintbits(result,64);
}
static OValue ostd_math_bit_floor(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("bit_floor expects a fixed-width unsigned integer");
  if(raw==0) return ouintbits(0,width);
  uint64_t result=UINT64_C(1); uint64_t cursor=raw;
  while(cursor>1) { cursor>>=1; result<<=1; }
  return ouintbits(result,width);
}
static OValue ostd_math_bit_ceil(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("bit_ceil expects a fixed-width unsigned integer");
  uint64_t result=UINT64_C(1);
  while(result<raw) {
    if(result>UINT64_MAX/2) return ofail("bit_ceil result is not representable in unsigned width");
    result<<=1;
  }
  if(result>omask(width)) return ofail("bit_ceil result is not representable in unsigned width");
  return ouintbits(result,width);
}
static OValue ostd_math_has_single_bit(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("has_single_bit expects a fixed-width unsigned integer");
  return obool(raw!=0 && (raw&(raw-1))==0);
}
static OValue ostd_math_byte_swap(OValue value) {
  uint64_t raw; uint16_t width;
  if(!ostd_math_bit_input(value,&raw,&width)) return ofail("byte_swap expects a fixed-width unsigned integer");
  if(width%8!=0) return ofail("byte_swap requires a byte-aligned unsigned width");
  uint16_t bytes=width/8; uint64_t result=0;
  for(uint16_t index=0; index<bytes; index++) { result=(result<<8)|(raw&UINT64_C(0xff)); raw>>=8; }
  return ouintbits(result,width);
}
static int ostd_string(OValue value, const char **data, size_t *len) { if(value.kind!=O_STRING) return 0; *data=value.as.s.data; *len=value.as.s.len; return 1; }
static int ostd_bytes(OValue value, const unsigned char **data, size_t *len, unsigned char **owned) {
  *owned=NULL;
  if(value.kind==O_STRING) { *data=(const unsigned char *)value.as.s.data; *len=value.as.s.len; return 1; }
  if(value.kind!=O_LIST) return 0;
  if(value.as.list.len>1048576) return 0;
  *owned=value.as.list.len?(unsigned char *)malloc(value.as.list.len):NULL;
  if(value.as.list.len&&!*owned) return 0;
  for(size_t i=0;i<value.as.list.len;i++) { if(value.as.list.items[i].kind!=O_UINT) { free(*owned); *owned=NULL; return 0; } (*owned)[i]=(unsigned char)(value.as.list.items[i].as.u&UINT64_C(0xff)); }
  *data=*owned; *len=value.as.list.len; return 1;
}
static OValue ostd_bytes_list(const unsigned char *data, size_t len) {
  if(len>1048576) return ofail("byte output exceeds the 1048576-byte limit");
  OValue *items=len?(OValue *)malloc(len*sizeof(OValue)):NULL; if(len&&!items) return ofail("out of memory");
  for(size_t i=0;i<len;i++) items[i]=ouintbits(data[i],8);
  OValue out=olist(items,len); free(items); return out;
}
static size_t ostd_runes(const char *data, size_t len) { size_t at=0,count=0; uint32_t r; while(at<len) { size_t before=at; if(!outf8decode(data,len,&at,&r)) return SIZE_MAX; if(at==before) return SIZE_MAX; count++; } return count; }
static int ostd_text_arg(OValue value, const char **data, size_t *len, const char *name) { if(!ostd_string(value,data,len)) { ofail(name); return 0; } if(*len>1048576) { ofail("text input exceeds the 1048576-byte limit"); return 0; } return 1; }
static OValue ostd_text_trim(OValue value, int left, int right) {
  const char *data; size_t len,at=0,start=0,end; uint32_t r;
  if(!ostd_text_arg(value,&data,&len,"text operation expects a string")) return onull(); end=len;
  if(left) { while(at<len) { size_t before=at; if(!outf8decode(data,len,&at,&r)) return ofail("text expects valid UTF-8"); if(!ounicodespace(r)) { start=before; break; } start=at; } }
  if(right) { at=start; size_t last=start; while(at<len) { size_t before=at; if(!outf8decode(data,len,&at,&r)) return ofail("text expects valid UTF-8"); if(!ounicodespace(r)) last=at; } end=last; }
  return oownedstring(data+start,end-start);
}
static int ostd_text_set_contains(const char *set, size_t len, uint32_t target) {
  size_t at=0; uint32_t r;
  while(at<len) { if(!outf8decode(set,len,&at,&r)) return -1; if(r==target) return 1; }
  return 0;
}
static OValue ostd_text_trim_set(OValue value, OValue set, int left) {
  const char *data,*chars; size_t len,charsLen;
  if(!ostd_text_arg(value,&data,&len,"trim character set expects strings")||!ostd_text_arg(set,&chars,&charsLen,"trim character set expects strings")) return onull();
  size_t start=0,end=len,at=0; uint32_t r;
  if(left) {
    while(at<len) { size_t before=at; if(!outf8decode(data,len,&at,&r)) return ofail("trim character set expects valid UTF-8"); int found=ostd_text_set_contains(chars,charsLen,r); if(found<0) return ofail("trim character set expects valid UTF-8"); if(!found) { start=before; break; } start=at; }
  } else {
    size_t last=0;
    while(at<len) { if(!outf8decode(data,len,&at,&r)) return ofail("trim character set expects valid UTF-8"); int found=ostd_text_set_contains(chars,charsLen,r); if(found<0) return ofail("trim character set expects valid UTF-8"); if(!found) last=at; }
    end=last;
  }
  return oownedstring(data+start,end-start);
}
static OValue ostd_text_case(OValue value, int upper) {
  const char *data; size_t len,at=0,n=0; uint32_t r; if(!ostd_text_arg(value,&data,&len,"case operation expects a string")) return onull();
  char *out=(char *)malloc(len*2+1); if(len&& !out) return ofail("out of memory");
  while(at<len) { if(!outf8decode(data,len,&at,&r)) { free(out); return ofail("text expects valid UTF-8"); } n+=outf8encode(out+n,upper?ounicodeupper(r):ounicodelower(r)); }
  OValue result=oownedstring(out,n); free(out); return result;
}
static uint32_t ostd_text_fold_rune(uint32_t r) {
  // Match Go's simple-fold equivalences that are not represented by the
  // compact lower-case table used by the portable runtime.
  if(r==0x3c2) return 0x3c3; // Greek final sigma -> sigma
  if(r==0x212a) return 'k';  // Kelvin sign -> latin k
  if(r==0x17f) return 's';   // long s -> latin s
  return ounicodelower(r);
}
static OValue ostd_text_equal_fold(OValue left, OValue right) {
  const char *a,*b; size_t an,bn,ai=0,bi=0; uint32_t ar,br;
  if(!ostd_text_arg(left,&a,&an,"equal_fold expects strings")||!ostd_text_arg(right,&b,&bn,"equal_fold expects strings")) return onull();
  while(ai<an&&bi<bn) {
    if(!outf8decode(a,an,&ai,&ar)||!outf8decode(b,bn,&bi,&br)) return ofail("text expects valid UTF-8");
    if(ostd_text_fold_rune(ar)!=ostd_text_fold_rune(br)) return obool(0);
  }
  return obool(ai==an&&bi==bn);
}
static OValue ostd_text_replace(OValue text, OValue old, OValue replacement, OValue limitValue) {
  const char *s,*o,*r; size_t sn,on,rn; int64_t limit; if(!ostd_string(text,&s,&sn)||!ostd_string(old,&o,&on)||!ostd_string(replacement,&r,&rn)) return ofail("replace expects strings");
  if(limitValue.kind!=O_NULL&&!ostd_i64(limitValue,&limit)) return ofail("replace limit must be an integer"); if(limitValue.kind==O_NULL) limit=-1; if(sn>1048576) return ofail("text input exceeds the 1048576-byte limit");
  if(on==0) { size_t count=ostd_runes(s,sn); if(count==SIZE_MAX) return ofail("text expects valid UTF-8"); size_t matches=count+1; if(limit>=0&&(uint64_t)limit<matches) matches=(size_t)limit; if(rn>0&&matches>(SIZE_MAX-sn)/rn) return ofail("text result is too large"); size_t outlen=sn+matches*rn; char *out=(char *)malloc(outlen+1); if(outlen&&!out) return ofail("out of memory"); size_t at=0,pos=0,done=0; if(limit!=0) { memcpy(out+at,r,rn); at+=rn; done++; } while(pos<sn) { uint32_t cp; size_t before=pos; if(!outf8decode(s,sn,&pos,&cp)) { free(out); return ofail("text expects valid UTF-8"); } memcpy(out+at,s+before,pos-before); at+=pos-before; if(limit<0||(int64_t)done<limit) { memcpy(out+at,r,rn); at+=rn; done++; } } OValue result=oownedstring(out,at); free(out); return result; }
  size_t matches=0,pos=0; while(pos+on<=sn) { const char *found=strstr(s+pos,o); if(!found) break; if(limit>=0&&(int64_t)matches>=limit) break; matches++; pos=(size_t)(found-s)+on; }
  if(rn>on&&matches>(SIZE_MAX-sn)/(rn-on)) return ofail("text result is too large"); size_t outlen=sn+matches*(rn-on); char *out=(char *)malloc(outlen+1); if(outlen&&!out) return ofail("out of memory"); size_t at=0,last=0; pos=0; while(pos+on<=sn&& (limit<0||(int64_t)matches>0)) { const char *found=strstr(s+pos,o); if(!found) break; if(limit>=0&&(int64_t)(matches-(size_t)0)==0) break; size_t p=(size_t)(found-s); if(p<last) break; memcpy(out+at,s+last,p-last); at+=p-last; memcpy(out+at,r,rn); at+=rn; last=p+on; pos=last; if(limit>=0) { if(--limit==0) break; } } memcpy(out+at,s+last,sn-last); at+=sn-last; OValue result=oownedstring(out,at); free(out); return result;
}
static OValue ostd_text_split(OValue text, OValue separator, int after, int64_t limit) {
  const char *s,*sep; size_t sn,dn; if(!ostd_string(text,&s,&sn)||!ostd_string(separator,&sep,&dn)) return ofail("split expects strings");
  OValue *items=NULL; size_t count=0,cap=0,from=0,parts=0;
  #define OSTD_PUSH_TEXT(ptr,n) do { if(count==cap) { size_t nc=cap?cap*2:8; OValue *ni=(OValue *)realloc(items,nc*sizeof(OValue)); if(!ni) { free(items); return ofail("out of memory"); } items=ni; cap=nc; } items[count++]=ostringview((ptr),(n)); } while(0)
  if(dn==0) { size_t at=0; uint32_t r; while(at<sn) { size_t before=at; if(!outf8decode(s,sn,&at,&r)) { free(items); return ofail("split expects valid UTF-8"); } if(limit>0&&parts+1==limit) { OSTD_PUSH_TEXT(s+before,sn-before); from=sn; break; } OSTD_PUSH_TEXT(s+before,at-before); parts++; from=at; } if(sn==0) { free(items); return olist(NULL,0); } if(from<sn) OSTD_PUSH_TEXT(s+from,sn-from); OValue out=olist(items,count); free(items); return out; }
  while(from<=sn) { if(limit>0&&parts+1==limit) { OSTD_PUSH_TEXT(s+from,sn-from); from=sn+1; break; } const char *found=strstr(s+from,sep); if(!found) { OSTD_PUSH_TEXT(s+from,sn-from); break; } size_t p=(size_t)(found-s),end=p+(after?dn:0); OSTD_PUSH_TEXT(s+from,end-from); from=p+dn; parts++; }
  OValue out=olist(items,count); free(items); return out;
  #undef OSTD_PUSH_TEXT
}
static OValue ostd_text_chunks(OValue text, OValue widthValue) {
  const char *s; size_t sn; int64_t width; if(!ostd_text_arg(text,&s,&sn,"chunks expects text and positive size")) return onull(); if(!ostd_i64(widthValue,&width)||width<=0) return ofail("chunks expects text and positive size");
  size_t *starts=NULL, count=0, cap=0, at=0; uint32_t r;
  while(at<sn) { if(count==cap) { size_t nc=cap?cap*2:8; size_t *ns=(size_t *)realloc(starts,nc*sizeof(size_t)); if(!ns) { free(starts); return ofail("out of memory"); } starts=ns; cap=nc; } starts[count++]=at; for(int64_t i=0;i<width&&at<sn;i++) { if(!outf8decode(s,sn,&at,&r)) { free(starts); return ofail("chunks expects valid UTF-8"); } } }
  OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) { free(starts); return ofail("out of memory"); } for(size_t i=0;i<count;i++) { size_t end=i+1<count?starts[i+1]:sn; items[i]=oownedstring(s+starts[i],end-starts[i]); if(ohaderror) { free(items); free(starts); return onull(); } } OValue out=olist(items,count); free(items); free(starts); return out;
}
static OValue ostd_text_join(OValue separator, OValue values) { if(separator.kind!=O_STRING||values.kind!=O_LIST) return ofail("join expects a string separator and list"); size_t len=0; for(size_t i=0;i<values.as.list.len;i++) { size_t part=ocatpart(NULL,0,values.as.list.items[i]); if(part>SIZE_MAX-len||(i&&separator.as.s.len>SIZE_MAX-len-part)) return ofail("join result is too large"); len+=part+(i?separator.as.s.len:0); } char *out=(char *)malloc(len+1); if(len&&!out) return ofail("out of memory"); size_t at=0; for(size_t i=0;i<values.as.list.len;i++) { if(i) { memcpy(out+at,separator.as.s.data,separator.as.s.len); at+=separator.as.s.len; } at+=ocatpart(out+at,len-at,values.as.list.items[i]); } OValue result=oownedstring(out,len); free(out); return result; }
static OValue ostd_text_fields(OValue value) { const char *s; size_t sn; if(!ostd_string(value,&s,&sn)) return ofail("fields expects a string"); OValue *items=NULL; size_t count=0,cap=0,at=0,start=0; uint32_t r; while(at<sn) { size_t before=at; if(!outf8decode(s,sn,&at,&r)) { free(items); return ofail("fields expects valid UTF-8"); } if(ounicodespace(r)) { if(start<before) { if(count==cap) { cap=cap?cap*2:8; items=(OValue *)realloc(items,cap*sizeof(OValue)); if(!items) return ofail("out of memory"); } items[count++]=ostringview(s+start,before-start); } start=at; } } if(start<sn) { if(count==cap) { cap=cap?cap*2:8; items=(OValue *)realloc(items,cap*sizeof(OValue)); if(!items) return ofail("out of memory"); } items[count++]=ostringview(s+start,sn-start); } OValue out=olist(items,count); free(items); return out; }
static OValue ostd_text_index(OValue text, OValue needle, int last, int any) {
  const char *s,*n; size_t sn,nn;
  if(!ostd_text_arg(text,&s,&sn,"text search expects strings")||!ostd_text_arg(needle,&n,&nn,"text search expects strings")) return onull();
  size_t p=SIZE_MAX;
  if(any) {
    size_t at=0; uint32_t a,b;
    while(at<sn) {
      size_t before=at; if(!outf8decode(s,sn,&at,&a)) return ofail("text expects valid UTF-8");
      size_t scan=0; while(scan<nn) { if(!outf8decode(n,nn,&scan,&b)) return ofail("text expects valid UTF-8"); if(a==b) { p=before; break; } }
      if(p!=SIZE_MAX&&!last) break;
    }
  } else if(nn==0) {
    p=last?sn:0;
  } else if(last) {
    const char *f=s; while((f=strstr(f,n))!=NULL) { p=(size_t)(f-s); f+=(nn?nn:1); }
  } else {
    const char *f=strstr(s,n); if(f) p=(size_t)(f-s);
  }
  if(p==SIZE_MAX) return ooptionNone();
  size_t runes=0,at=0; uint32_t r; while(at<p) { if(!outf8decode(s,sn,&at,&r)) return ofail("text expects valid UTF-8"); runes++; }
  return ooptionSome(ouint(runes));
}
static OValue ostd_text_count(OValue text, OValue needle) {
  const char *s,*n; size_t sn,nn;
  if(!ostd_text_arg(text,&s,&sn,"count expects strings")||!ostd_text_arg(needle,&n,&nn,"count expects strings")) return onull();
  if(nn==0) { size_t runes=ostd_runes(s,sn); return runes==SIZE_MAX?ofail("text expects valid UTF-8"):ouint(runes+1); }
  size_t count=0,at=0; while(at<=sn) { const char *found=strstr(s+at,n); if(!found) break; count++; at=(size_t)(found-s)+nn; }
  return ouint(count);
}
static OValue ostd_list_take(OValue value, int64_t n) { if(n<0) return ofail("take count must be non-negative"); if(value.kind==O_STRING) { size_t at=0,count=0; uint32_t r; while(at<value.as.s.len&&count<(size_t)n) { if(!outf8decode(value.as.s.data,value.as.s.len,&at,&r)) return ofail("take expects valid UTF-8"); count++; } return oownedstring(value.as.s.data,at); } if(value.kind==O_LIST) { size_t count=value.as.list.len<(uint64_t)n?value.as.list.len:(size_t)n; return olist(value.as.list.items,count); } if(value.kind==O_TENSOR) { size_t count=value.as.tensor.len<(uint64_t)n?value.as.tensor.len:(size_t)n, shape[1]={count}; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); if(count) memcpy(data,value.as.tensor.data,count*sizeof(double)); return otensordata(data,count,shape,1); } return ofail("take expects a string, list, or tensor"); }
static OValue ostd_list_range(OValue value) { int64_t n; if(!ostd_i64(value,&n)||n<0||n>100000) return ofail("range expects a non-negative bounded integer"); OValue *items=n?(OValue *)malloc((size_t)n*sizeof(OValue)):NULL; if(n&&!items) return ofail("out of memory"); for(int64_t i=0;i<n;i++) items[i]=ofloat((double)i); OValue out=olist(items,(size_t)n); free(items); return out; }
static OValue ostd_list_reverse(OValue value) { if(value.kind!=O_LIST) return ofail("reverse expects a list"); OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL; if(value.as.list.len&&!items) return ofail("out of memory"); for(size_t i=0;i<value.as.list.len;i++) items[i]=value.as.list.items[value.as.list.len-1-i]; OValue out=olist(items,value.as.list.len); free(items); return out; }
static int ostd_list_flatten_append(OValue value, int depth, OValue *items, size_t *count) {
  if(value.kind!=O_LIST || depth==0) {
    if(*count>=100000) { ofail("std/list.flatten: output items 100001 exceeds limit 100000"); return 0; }
    items[(*count)++]=value;
    return 1;
  }
  for(size_t i=0;i<value.as.list.len;i++) {
    if(!ostd_list_flatten_append(value.as.list.items[i],depth-1,items,count)) return 0;
  }
  return 1;
}
static OValue ostd_list_flatten(OValue value, OValue depthValue) {
  int64_t depth;
  if(value.kind!=O_LIST||!ostd_i64(depthValue,&depth)||depth<0||depth>64) return ofail("flatten expects a list and depth in [0,64]");
  if(value.as.list.len>100000) return ofail("std/list.flatten: input items exceed limit 100000");
  OValue *items=100000?(OValue *)malloc(100000*sizeof(OValue)):NULL;
  if(!items) return ofail("out of memory");
  size_t count=0;
  for(size_t i=0;i<value.as.list.len;i++) {
    if(!ostd_list_flatten_append(value.as.list.items[i],(int)depth,items,&count)) { free(items); return onull(); }
  }
  OValue out=olist(items,count);
  free(items);
  return out;
}
static OValue ostd_list_fold(OValue value, int product) {
  if(value.kind!=O_LIST) return ofail(product?"product expects a list":"sum expects a list");
  OComplex total=product?(OComplex){1,0}:(OComplex){0,0};
  uint16_t bits=0; int complex_result=0;
  for(size_t i=0;i<value.as.list.len;i++) {
    OComplex item;
    if(!ocomplexnum(value.as.list.items[i],&item)) return ofail(product?"product expects numeric values":"sum expects numeric values");
    if(value.as.list.items[i].kind==O_COMPLEX) complex_result=1;
    if(value.as.list.items[i].kind==O_FLOAT&&value.as.list.items[i].fbits>bits) bits=value.as.list.items[i].fbits;
    total=product?(OComplex){total.real*item.real-total.imag*item.imag,total.real*item.imag+total.imag*item.real}:(OComplex){total.real+item.real,total.imag+item.imag};
  }
  if(complex_result) return ocomplexbits(total.real,total.imag,bits);
  return ofloatbits(total.real,bits);
}
static OValue ostd_list_sum(OValue value) { return ostd_list_fold(value,0); }
static OValue ostd_list_product(OValue value) { return ostd_list_fold(value,1); }
static int ostd_list_bounded(OValue value, const char *message) { if(value.kind!=O_LIST) { ofail(message); return 0; } if(value.as.list.len>100000) { ofail("list input exceeds limit 100000"); return 0; } return 1; }
static OValue ostd_list_at(OValue value, OValue index) { int64_t i; if(!ostd_list_bounded(value,"at expects a list and non-negative index")) return onull(); if(!ostd_i64(index,&i)||i<0) return ofail("at expects a list and non-negative index"); if((uint64_t)i>=(uint64_t)value.as.list.len) return ofail("list index out of range"); return value.as.list.items[(size_t)i]; }
static OValue ostd_list_index(OValue value, OValue target, int last, int wantCount) { if(!ostd_list_bounded(value,"index expects a list")) return onull(); int64_t found=-1,count=0; for(size_t i=0;i<value.as.list.len;i++) { OValue eq=oequal(value.as.list.items[i],target); if(eq.kind==O_ERROR) return eq; if(eq.kind==O_BOOL&&eq.as.b) { count++; found=(int64_t)i; if(!last&&!wantCount) break; } } return wantCount?ouint((uint64_t)count):oint(found); }
static OValue ostd_list_sort(OValue value) { if(!ostd_list_bounded(value,"sort expects a list")) return onull(); OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL; if(value.as.list.len&&!items) return ofail("out of memory"); if(value.as.list.len) memcpy(items,value.as.list.items,value.as.list.len*sizeof(OValue)); for(size_t i=1;i<value.as.list.len;i++) { OValue item=items[i]; size_t j=i; while(j>0) { OValue less=obin("<",item,items[j-1]); if(less.kind==O_ERROR) { free(items); return less; } if(!less.as.b) break; items[j]=items[j-1]; j--; } items[j]=item; } OValue out=olist(items,value.as.list.len); free(items); return out; }
static OValue ostd_list_unique(OValue value) { if(!ostd_list_bounded(value,"unique expects a list")) return onull(); OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL; if(value.as.list.len&&!items) return ofail("out of memory"); size_t n=0; for(size_t i=0;i<value.as.list.len;i++) { int seen=0; for(size_t j=0;j<n;j++) { OValue eq=oequal(items[j],value.as.list.items[i]); if(eq.kind==O_ERROR) { free(items); return eq; } if(eq.as.b) { seen=1; break; } } if(!seen) items[n++]=value.as.list.items[i]; } OValue out=olist(items,n); free(items); return out; }
static OValue ostd_list_partition(OValue value, OValue indexValue) {
  int64_t index;
  if(!ostd_list_bounded(value,"partition expects a list and non-negative index")) return onull();
  if(!ostd_i64(indexValue,&index)||index<0) return ofail("partition expects a list and non-negative index");
  if((uint64_t)index>(uint64_t)value.as.list.len) return ofail("std/list.partition: std/list.slice: require 0 <= start <= end");
  size_t split=(size_t)index;
  OValue left=olist(value.as.list.items,split);
  if(left.kind==O_ERROR) return left;
  OValue right=olist(value.as.list.items+split,value.as.list.len-split);
  if(right.kind==O_ERROR) return right;
  OValue pair[2]={left,right};
  OValue out=olist(pair,2);
  if(out.kind==O_LIST) out.fbits=1;
  return out;
}
static OValue ostd_list_zip(OValue left, OValue right) { if(!ostd_list_bounded(left,"zip expects two lists")||!ostd_list_bounded(right,"zip expects two lists")) return onull(); size_t n=left.as.list.len<right.as.list.len?left.as.list.len:right.as.list.len; OValue *items=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!items) return ofail("out of memory"); for(size_t i=0;i<n;i++) { OValue pair[2]={left.as.list.items[i],right.as.list.items[i]}; items[i]=olist(pair,2); if(items[i].kind==O_LIST) items[i].fbits=1; } OValue out=olist(items,n); free(items); return out; }
static OValue ostd_list_shape(OValue value, int mode, int64_t a, int64_t b) {
  if(mode==1) {
    if(!ostd_list_bounded(value,"chunks expects a list")) return onull();
    if(a<=0||a>100000) return ofail("chunks width must be positive and within the item limit");
    size_t width=(size_t)a;
    size_t count=(value.as.list.len+width-1)/width;
    OValue *out=count?(OValue *)malloc(count*sizeof(OValue)):NULL;
    if(count&&!out) return ofail("out of memory");
    for(size_t i=0;i<count;i++) {
      size_t p=i*width;
      size_t n=value.as.list.len-p<width?value.as.list.len-p:width;
      out[i]=olist(value.as.list.items+p,n);
      if(out[i].kind==O_ERROR) { free(out); return out[i]; }
    }
    OValue result=olist(out,count);
    free(out);
    return result;
  }
  if(value.kind!=O_LIST||a<0||b<0||a>b) return ofail("list shape arguments are invalid");
  size_t start=(size_t)a,end=(size_t)b;
  if(start>value.as.list.len) start=value.as.list.len;
  if(end>value.as.list.len) end=value.as.list.len;
  if(mode==0) return olist(value.as.list.items+start,end-start);
  size_t width=(size_t)a;
  if(mode==2&&width==0) return ofail("window width must be positive");
  size_t step=(size_t)b;
  if(mode==2&&step==0) return ofail("window step must be positive");
  size_t count=value.as.list.len>=width?(value.as.list.len-width)/step+1:0;
  OValue *out=count?(OValue *)malloc(count*sizeof(OValue)):NULL;
  if(count&&!out) return ofail("out of memory");
  for(size_t i=0;i<count;i++) {
    size_t p=i*step;
    size_t n=width;
    out[i]=olist(value.as.list.items+p,n);
  }
  OValue result=olist(out,count);
  free(out);
  return result;
}
static OValue ostd_signal_window(OValue value, int kind) { int64_t n; if(!ostd_i64(value,&n)||n<0||n>65536) return ofail("signal window length is out of range"); size_t count=(size_t)n; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t i=0;i<count;i++) { double x=count>1?(double)i/(double)(count-1):0,angle=2*3.14159265358979323846*x; data[i]=count<=1?1.0:(kind==0?1.0:kind==1?0.5-0.5*ogocos(angle):kind==2?0.54-0.46*ogocos(angle):kind==3?0.42-0.5*ogocos(angle)+0.08*ogocos(2*angle):1.0-fabs(2*x-1.0)); } size_t shape[1]={count}; return otensordata(data,count,shape,1); }
static int ostd_signal_data(OValue value, double **out, size_t *n) { if(value.kind==O_TENSOR&&value.as.tensor.rank==1) { *n=value.as.tensor.len; *out=(double *)malloc(*n*sizeof(double)); if(*n&&!*out) return 0; if(*n) memcpy(*out,value.as.tensor.data,*n*sizeof(double)); return 1; } if(value.kind!=O_LIST) return 0; *n=value.as.list.len; *out=*n?(double *)malloc(*n*sizeof(double)):NULL; if(*n&&!*out) return 0; for(size_t i=0;i<*n;i++) if(!onum(value.as.list.items[i],&(*out)[i])) { free(*out); *out=NULL; return 0; } return 1; }
static OValue ostd_signal_resample(OValue value, OValue lengthValue) {
  size_t inputCount;
  if(value.kind==O_TENSOR) {
    if(value.as.tensor.rank!=1) return ofail("signal.resample expects a one-dimensional numeric signal");
    inputCount=value.as.tensor.len;
  } else if(value.kind==O_LIST) {
    inputCount=value.as.list.len;
  } else {
    return ofail("signal.resample expects a one-dimensional numeric signal");
  }
  if(inputCount>65536) return ofail("signal.resample exceeds the 65536-sample limit");
  double *data=NULL; size_t n=0;
  if(!ostd_signal_data(value,&data,&n)) return ofail("signal.resample expects a one-dimensional numeric signal");
  int64_t requested;
  if(!ostd_i64(lengthValue,&requested)||requested<0||requested>65536) { free(data); return ofail("signal.resample length must be an integer from 0 through 65536"); }
  size_t count=(size_t)requested, shape[1]={count};
  double *out=count?(double *)malloc(count*sizeof(double)):NULL;
  if(count&&!out) { free(data); return ofail("out of memory"); }
  if(count==0) { free(data); return otensordata(NULL,0,shape,1); }
  if(n==0) {
    for(size_t i=0;i<count;i++) out[i]=0;
  } else if(n==1) {
    for(size_t i=0;i<count;i++) out[i]=data[0];
  } else if(count==1) {
    out[0]=data[0];
  } else {
    for(size_t i=0;i<count;i++) {
      double position=(double)i*(double)(n-1)/(double)(count-1);
      size_t left=(size_t)floor(position), right=left+1;
      if(right>=n) right=n-1;
      double fraction=position-(double)left;
      out[i]=data[left]+fraction*(data[right]-data[left]);
    }
  }
  free(data);
  return otensordata(out,count,shape,1);
}
static OValue ostd_signal_conv(OValue left, OValue right, int correlate) { double *a,*b; size_t an,bn; if(!ostd_signal_data(left,&a,&an)||!ostd_signal_data(right,&b,&bn)) { free(a); free(b); return ofail("signal expects rank-one numeric data"); } size_t count=an&&bn?an+bn-1:0; double *out=count?(double *)calloc(count,sizeof(double)):NULL; if(count&&!out) { free(a); free(b); return ofail("out of memory"); } for(size_t i=0;i<count;i++) for(size_t j=0;j<an;j++) { size_t k=i>=j?i-j:SIZE_MAX; if(k<bn) out[i]+=a[j]*(correlate?b[bn-1-k]:b[k]); } free(a); free(b); size_t shape[1]={count}; return otensordata(out,count,shape,1); }
static OValue ostd_signal_fir(OValue value, OValue coefficients) { double *a,*b; size_t an,bn; if(!ostd_signal_data(value,&a,&an)||!ostd_signal_data(coefficients,&b,&bn)) { free(a); free(b); return ofail("signal expects rank-one numeric data"); } double *out=an?(double *)calloc(an,sizeof(double)):NULL; if(an&&!out) { free(a); free(b); return ofail("out of memory"); } for(size_t i=0;i<an;i++) for(size_t j=0;j<bn&&j<=i;j++) out[i]+=a[i-j]*b[j]; free(a); free(b); size_t shape[1]={an}; return otensordata(out,an,shape,1); }
static OValue ostd_signal_stats(OValue value, int mode) { double *data; size_t n; if(!ostd_signal_data(value,&data,&n)||n==0) { free(data); return ofail("signal statistics expects non-empty numeric data"); } double sum=0,energy=0; for(size_t i=0;i<n;i++) { sum+=data[i]; energy+=data[i]*data[i]; } double mean=sum/(double)n,result; if(mode==0) result=mean; else if(mode==1) result=energy; else if(mode==2) { double variance=0; for(size_t i=0;i<n;i++) { double delta=data[i]-mean; variance+=delta*delta; } result=variance/(double)n; } else result=sqrt(energy/(double)n); free(data); return ofloat(result); }
static OValue ostd_signal_normalize(OValue value, OValue modeValue) { if(modeValue.kind!=O_STRING) return ofail("signal.normalize mode must be a string"); double *data; size_t n; if(!ostd_signal_data(value,&data,&n)) return ofail("signal.normalize expects rank-one numeric data"); int mode=!strcmp(modeValue.as.s.data,"peak")?0:!strcmp(modeValue.as.s.data,"minmax")?1:!strcmp(modeValue.as.s.data,"zscore")?2:-1; if(mode<0) { free(data); return ofail("signal.normalize mode must be peak, minmax, or zscore"); } if(n==0) { size_t shape[1]={0}; free(data); return otensordata(NULL,0,shape,1); } if(mode==0) { double peak=0; for(size_t i=0;i<n;i++) if(fabs(data[i])>peak) peak=fabs(data[i]); if(peak==0) for(size_t i=0;i<n;i++) data[i]=0; else for(size_t i=0;i<n;i++) data[i]/=peak; } else if(mode==1) { double lo=data[0],hi=data[0]; for(size_t i=1;i<n;i++) { if(data[i]<lo) lo=data[i]; if(data[i]>hi) hi=data[i]; } if(hi==lo) for(size_t i=0;i<n;i++) data[i]=0; else for(size_t i=0;i<n;i++) data[i]=(data[i]-lo)/(hi-lo); } else { double sum=0; for(size_t i=0;i<n;i++) sum+=data[i]; double mean=sum/(double)n,variance=0; for(size_t i=0;i<n;i++) { double delta=data[i]-mean; variance+=delta*delta; } double deviation=sqrt(variance/(double)n); if(deviation==0) for(size_t i=0;i<n;i++) data[i]=0; else for(size_t i=0;i<n;i++) data[i]=(data[i]-mean)/deviation; } size_t shape[1]={n}; return otensordata(data,n,shape,1); }
static int ostd_signal_complex_data(OValue value, OComplex **out, size_t *n) {
  *out=NULL; *n=0;
  if(value.kind==O_COMPLEX_TENSOR) {
    if(value.as.ctensor.rank!=1||value.as.ctensor.len>4096) return 0;
    *n=value.as.ctensor.len;
    *out=*n?(OComplex *)malloc(*n*sizeof(OComplex)):NULL;
    if(*n&&!*out) return 0;
    if(*n) memcpy(*out,value.as.ctensor.data,*n*sizeof(OComplex));
    return 1;
  }
  if(value.kind==O_TENSOR) {
    if(value.as.tensor.rank!=1||value.as.tensor.len>4096) return 0;
    *n=value.as.tensor.len;
    *out=*n?(OComplex *)malloc(*n*sizeof(OComplex)):NULL;
    if(*n&&!*out) return 0;
    for(size_t i=0;i<*n;i++) (*out)[i]=(OComplex){value.as.tensor.data[i],0};
    return 1;
  }
  if(value.kind!=O_LIST||value.as.list.len>4096) return 0;
  *n=value.as.list.len;
  *out=*n?(OComplex *)malloc(*n*sizeof(OComplex)):NULL;
  if(*n&&!*out) return 0;
  for(size_t i=0;i<*n;i++) {
    if(!ocomplexnum(value.as.list.items[i],&(*out)[i])) { free(*out); *out=NULL; *n=0; return 0; }
  }
  return 1;
}
static OValue ostd_signal_fft(OValue value, int inverse) {
  OComplex *data; size_t n;
  if(!ostd_signal_complex_data(value,&data,&n)) return ofail("fft expects a rank-one numeric signal within the 4096-sample limit");
  OComplex *items=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL;
  if(n&&!items) { free(data); return ofail("out of memory"); }
  for(size_t k=0;k<n;k++) {
    double re=0,im=0;
    for(size_t t=0;t<n;t++) {
      double angle=(inverse?1:-1)*2*3.14159265358979323846*(double)k*(double)t/(double)(n?n:1);
      double c=ogocos(angle),s=ogosin(angle);
      re+=data[t].real*c-data[t].imag*s;
      im+=data[t].real*s+data[t].imag*c;
    }
    if(inverse) { re/=n?n:1; im/=n?n:1; }
    items[k]=(OComplex){re,im};
  }
  free(data);
  size_t shape[1]={n};
  return ocomplextensordata(items,n,shape,1);
}
typedef struct { uint64_t h[8]; uint64_t bits_hi, bits_lo; unsigned char block[128]; size_t used; } OSHA512;
static uint64_t osh512rotr(uint64_t x, unsigned n) { return (x>>n)|(x<<(64-n)); }
static uint64_t osh512be64(const unsigned char *p) { return ((uint64_t)p[0]<<56)|((uint64_t)p[1]<<48)|((uint64_t)p[2]<<40)|((uint64_t)p[3]<<32)|((uint64_t)p[4]<<24)|((uint64_t)p[5]<<16)|((uint64_t)p[6]<<8)|p[7]; }
static void osh512put64(unsigned char *p, uint64_t x) { for(int i=7;i>=0;i--){p[i]=(unsigned char)x;x>>=8;} }
static void osh512block(OSHA512 *s, const unsigned char *p) {
  static const uint64_t k[80]={
    UINT64_C(0x428a2f98d728ae22),UINT64_C(0x7137449123ef65cd),UINT64_C(0xb5c0fbcfec4d3b2f),UINT64_C(0xe9b5dba58189dbbc),UINT64_C(0x3956c25bf348b538),UINT64_C(0x59f111f1b605d019),UINT64_C(0x923f82a4af194f9b),UINT64_C(0xab1c5ed5da6d8118),
    UINT64_C(0xd807aa98a3030242),UINT64_C(0x12835b0145706fbe),UINT64_C(0x243185be4ee4b28c),UINT64_C(0x550c7dc3d5ffb4e2),UINT64_C(0x72be5d74f27b896f),UINT64_C(0x80deb1fe3b1696b1),UINT64_C(0x9bdc06a725c71235),UINT64_C(0xc19bf174cf692694),
    UINT64_C(0xe49b69c19ef14ad2),UINT64_C(0xefbe4786384f25e3),UINT64_C(0x0fc19dc68b8cd5b5),UINT64_C(0x240ca1cc77ac9c65),UINT64_C(0x2de92c6f592b0275),UINT64_C(0x4a7484aa6ea6e483),UINT64_C(0x5cb0a9dcbd41fbd4),UINT64_C(0x76f988da831153b5),
    UINT64_C(0x983e5152ee66dfab),UINT64_C(0xa831c66d2db43210),UINT64_C(0xb00327c898fb213f),UINT64_C(0xbf597fc7beef0ee4),UINT64_C(0xc6e00bf33da88fc2),UINT64_C(0xd5a79147930aa725),UINT64_C(0x06ca6351e003826f),UINT64_C(0x142929670a0e6e70),
    UINT64_C(0x27b70a8546d22ffc),UINT64_C(0x2e1b21385c26c926),UINT64_C(0x4d2c6dfc5ac42aed),UINT64_C(0x53380d139d95b3df),UINT64_C(0x650a73548baf63de),UINT64_C(0x766a0abb3c77b2a8),UINT64_C(0x81c2c92e47edaee6),UINT64_C(0x92722c851482353b),
    UINT64_C(0xa2bfe8a14cf10364),UINT64_C(0xa81a664bbc423001),UINT64_C(0xc24b8b70d0f89791),UINT64_C(0xc76c51a30654be30),UINT64_C(0xd192e819d6ef5218),UINT64_C(0xd69906245565a910),UINT64_C(0xf40e35855771202a),UINT64_C(0x106aa07032bbd1b8),
    UINT64_C(0x19a4c116b8d2d0c8),UINT64_C(0x1e376c085141ab53),UINT64_C(0x2748774cdf8eeb99),UINT64_C(0x34b0bcb5e19b48a8),UINT64_C(0x391c0cb3c5c95a63),UINT64_C(0x4ed8aa4ae3418acb),UINT64_C(0x5b9cca4f7763e373),UINT64_C(0x682e6ff3d6b2b8a3),
    UINT64_C(0x748f82ee5defb2fc),UINT64_C(0x78a5636f43172f60),UINT64_C(0x84c87814a1f0ab72),UINT64_C(0x8cc702081a6439ec),UINT64_C(0x90befffa23631e28),UINT64_C(0xa4506cebde82bde9),UINT64_C(0xbef9a3f7b2c67915),UINT64_C(0xc67178f2e372532b),
    UINT64_C(0xca273eceea26619c),UINT64_C(0xd186b8c721c0c207),UINT64_C(0xeada7dd6cde0eb1e),UINT64_C(0xf57d4f7fee6ed178),UINT64_C(0x06f067aa72176fba),UINT64_C(0x0a637dc5a2c898a6),UINT64_C(0x113f9804bef90dae),UINT64_C(0x1b710b35131c471b),
    UINT64_C(0x28db77f523047d84),UINT64_C(0x32caab7b40c72493),UINT64_C(0x3c9ebe0a15c9bebc),UINT64_C(0x431d67c49c100d4c),UINT64_C(0x4cc5d4becb3e42b6),UINT64_C(0x597f299cfc657e2a),UINT64_C(0x5fcb6fab3ad6faec),UINT64_C(0x6c44198c4a475817)
  };
  uint64_t w[80]; for(size_t i=0;i<16;i++) w[i]=osh512be64(p+i*8);
  for(size_t i=16;i<80;i++){uint64_t a=osh512rotr(w[i-15],1)^osh512rotr(w[i-15],8)^(w[i-15]>>7);uint64_t b=osh512rotr(w[i-2],19)^osh512rotr(w[i-2],61)^(w[i-2]>>6);w[i]=w[i-16]+a+w[i-7]+b;}
  uint64_t a=s->h[0],b=s->h[1],c=s->h[2],d=s->h[3],e=s->h[4],f=s->h[5],g=s->h[6],h=s->h[7];
  for(size_t i=0;i<80;i++){uint64_t s1=osh512rotr(e,14)^osh512rotr(e,18)^osh512rotr(e,41);uint64_t ch=(e&f)^((~e)&g);uint64_t t1=h+s1+ch+k[i]+w[i];uint64_t s0=osh512rotr(a,28)^osh512rotr(a,34)^osh512rotr(a,39);uint64_t maj=(a&b)^(a&c)^(b&c);uint64_t t2=s0+maj;h=g;g=f;f=e;e=d+t1;d=c;c=b;b=a;a=t1+t2;}
  s->h[0]+=a;s->h[1]+=b;s->h[2]+=c;s->h[3]+=d;s->h[4]+=e;s->h[5]+=f;s->h[6]+=g;s->h[7]+=h;
}
static void osh512init(OSHA512 *s, int truncated256) { static const uint64_t full[8]={UINT64_C(0x6a09e667f3bcc908),UINT64_C(0xbb67ae8584caa73b),UINT64_C(0x3c6ef372fe94f82b),UINT64_C(0xa54ff53a5f1d36f1),UINT64_C(0x510e527fade682d1),UINT64_C(0x9b05688c2b3e6c1f),UINT64_C(0x1f83d9abfb41bd6b),UINT64_C(0x5be0cd19137e2179)}; static const uint64_t trunc[8]={UINT64_C(0x22312194fc2bf72c),UINT64_C(0x9f555fa3c84c64c2),UINT64_C(0x2393b86b6f53b151),UINT64_C(0x963877195940eabd),UINT64_C(0x96283ee2a88effe3),UINT64_C(0xbe5e1e2553863992),UINT64_C(0x2b0199fc2c85b8aa),UINT64_C(0x0eb72ddc81c52ca2)}; memcpy(s->h,truncated256?trunc:full,sizeof(s->h));s->bits_hi=0;s->bits_lo=0;s->used=0; }
static void osh512write(OSHA512 *s, const unsigned char *data, size_t len) { uint64_t add=(uint64_t)len*8;uint64_t old=s->bits_lo;s->bits_lo+=add;if(s->bits_lo<old)s->bits_hi++;s->bits_hi+=(uint64_t)len>>(64-3);while(len){size_t take=128-s->used;if(take>len)take=len;memcpy(s->block+s->used,data,take);s->used+=take;data+=take;len-=take;if(s->used==128){osh512block(s,s->block);s->used=0;}} }
static void osh512final(OSHA512 *s, unsigned char out[64], int truncated256) { s->block[s->used++]=0x80;if(s->used>112){while(s->used<128)s->block[s->used++]=0;osh512block(s,s->block);s->used=0;}while(s->used<112)s->block[s->used++]=0;osh512put64(s->block+112,s->bits_hi);osh512put64(s->block+120,s->bits_lo);osh512block(s,s->block);size_t n=truncated256?4:8;for(size_t i=0;i<n;i++)osh512put64(out+i*8,s->h[i]); }
static OValue ostd_crypto_digest(OValue dataValue, OValue algorithm, int raw) { const char *name; size_t nl; const unsigned char *data; size_t len; unsigned char *owned=NULL; if(!ostd_string(algorithm,&name,&nl)||!ostd_bytes(dataValue,&data,&len,&owned)) { free(owned); return ofail("digest expects byte-compatible data and algorithm"); } int sha512=nl==6&&!memcmp(name,"sha512",6); int sha512_256=nl==10&&!memcmp(name,"sha512_256",10); if(!sha512&& !sha512_256&&!(nl==6&&!memcmp(name,"sha256",6))) { free(owned); return ofail("unsupported crypto algorithm"); } unsigned char digest[64]; size_t digestLen=32; if(sha512||sha512_256){OSHA512 state;osh512init(&state,sha512_256);osh512write(&state,data,len);osh512final(&state,digest,sha512_256);digestLen=sha512_256?32:64;}else{OSHA256 state;oshainit(&state);oshawrite(&state,data,len);oshafinal(&state,digest);} free(owned); if(raw) return ostd_bytes_list(digest,digestLen); char hex[129]; static const char d[]="0123456789abcdef"; for(size_t i=0;i<digestLen;i++){hex[2*i]=d[digest[i]>>4];hex[2*i+1]=d[digest[i]&15];} return oownedstring(hex,digestLen*2); }
static OValue ostd_crypto_hmac(OValue keyValue, OValue dataValue, OValue algorithm) {
  const char *name; size_t nl; const unsigned char *key,*data; size_t keyLen,dataLen; unsigned char *ownedKey=NULL,*ownedData=NULL;
  if(!ostd_string(algorithm,&name,&nl)) return ofail("crypto.hmac expects key, data, and algorithm");
  if((keyValue.kind==O_STRING&&keyValue.as.s.len>65536)||(keyValue.kind==O_LIST&&keyValue.as.list.len>65536)) return ofail("crypto.hmac key exceeds the 65536-byte limit");
  if((dataValue.kind==O_STRING&&dataValue.as.s.len>1048576)||(dataValue.kind==O_LIST&&dataValue.as.list.len>1048576)) return ofail("crypto.hmac data exceeds the 1048576-byte limit");
  if(!ostd_bytes(keyValue,&key,&keyLen,&ownedKey)||!ostd_bytes(dataValue,&data,&dataLen,&ownedData)) { free(ownedKey); free(ownedData); return ofail("crypto.hmac expects byte-compatible key and data"); }
  int sha512=nl==6&&!memcmp(name,"sha512",6), sha512_256=nl==10&&!memcmp(name,"sha512_256",10), sha256=nl==6&&!memcmp(name,"sha256",6);
  if(!sha256&&!sha512&&!sha512_256) { free(ownedKey); free(ownedData); return ofail("unsupported crypto algorithm"); }
  size_t blockSize=sha256?64:128, digestLen=sha512_256?32:(sha256?32:64); unsigned char block[128],ipad[128],opad[128],inner[64],outer[64];
  memset(block,0,sizeof(block));
  if(keyLen>blockSize) {
    if(sha256) { OSHA256 state; oshainit(&state); oshawrite(&state,key,keyLen); oshafinal(&state,block); }
    else { OSHA512 state; osh512init(&state,sha512_256); osh512write(&state,key,keyLen); osh512final(&state,block,sha512_256); }
  } else if(keyLen) memcpy(block,key,keyLen);
  for(size_t i=0;i<blockSize;i++){ ipad[i]=(unsigned char)(block[i]^0x36); opad[i]=(unsigned char)(block[i]^0x5c); }
  if(sha256) { OSHA256 state; oshainit(&state); oshawrite(&state,ipad,blockSize); oshawrite(&state,data,dataLen); oshafinal(&state,inner); oshainit(&state); oshawrite(&state,opad,blockSize); oshawrite(&state,inner,32); oshafinal(&state,outer); }
  else { OSHA512 state; osh512init(&state,sha512_256); osh512write(&state,ipad,blockSize); osh512write(&state,data,dataLen); osh512final(&state,inner,sha512_256); osh512init(&state,sha512_256); osh512write(&state,opad,blockSize); osh512write(&state,inner,digestLen); osh512final(&state,outer,sha512_256); }
  free(ownedKey); free(ownedData); char hex[129]; static const char digits[]="0123456789abcdef"; for(size_t i=0;i<digestLen;i++){hex[2*i]=digits[outer[i]>>4];hex[2*i+1]=digits[outer[i]&15];} return oownedstring(hex,digestLen*2);
}
static OValue ostd_crypto_hex(OValue value, int decode) {
  const char *s; size_t n; const unsigned char *data; unsigned char *owned=NULL;
  if(decode) {
    if(!ostd_string(value,&s,&n)) return ofail("hex_decode expects a string");
    if(n%2) return ofail("hex_decode expects an even-length string");
    if(n>2097152) return ofail("hex_decode exceeds the 1048576-byte output limit");
    unsigned char *out=n?(unsigned char *)malloc(n/2):NULL;
    if(n&&!out) return ofail("out of memory");
    for(size_t i=0;i<n;i+=2) {
      int a=(s[i]>='0'&&s[i]<='9')?s[i]-'0':(s[i]>='a'&&s[i]<='f')?s[i]-'a'+10:(s[i]>='A'&&s[i]<='F')?s[i]-'A'+10:-1;
      int b=(s[i+1]>='0'&&s[i+1]<='9')?s[i+1]-'0':(s[i+1]>='a'&&s[i+1]<='f')?s[i+1]-'a'+10:(s[i+1]>='A'&&s[i+1]<='F')?s[i+1]-'A'+10:-1;
      if(a<0||b<0) { free(out); return ofail("invalid hexadecimal encoding"); }
      out[i/2]=(unsigned char)((a<<4)|b);
    }
    OValue result=ostd_bytes_list(out,n/2); free(out); return result;
  }
  if(!ostd_bytes(value,&data,&n,&owned)) return ofail("hex_encode expects byte-compatible data");
  if(n>1048576) { free(owned); return ofail("hex_encode exceeds the 1048576-byte input limit"); }
  char *out=n?(char *)malloc(n*2+1):NULL;
  if(n&&!out) { free(owned); return ofail("out of memory"); }
  static const char d[]="0123456789abcdef";
  for(size_t i=0;i<n;i++){out[2*i]=d[data[i]>>4];out[2*i+1]=d[data[i]&15];}
  OValue result=oownedstring(out,n*2); free(out); free(owned); return result;
}
static OValue ostd_crypto_compare_digest(OValue left, OValue right) {
  const unsigned char *a,*b; size_t an,bn; unsigned char *oa=NULL,*ob=NULL;
  if(!ostd_bytes(left,&a,&an,&oa)||!ostd_bytes(right,&b,&bn,&ob)) {
    free(oa); free(ob); return ofail("compare_digest expects byte-compatible values");
  }
  if(an>1048576||bn>1048576) {
    free(oa); free(ob); return ofail("compare_digest input exceeds the 1048576-byte limit");
  }
  unsigned char diff=(unsigned char)(an==bn?0:1);
  size_t n=an<bn?an:bn;
  for(size_t i=0;i<n;i++) diff|=a[i]^b[i];
  free(oa); free(ob); return obool(diff==0);
}
static int ostd_crypto_base64_value(unsigned char c, int url) {
  if(c>='A'&&c<='Z') return (int)(c-'A');
  if(c>='a'&&c<='z') return (int)(c-'a'+26);
  if(c>='0'&&c<='9') return (int)(c-'0'+52);
  if(c==(url?'-':'+')) return 62;
  if(c==(url?'_':'/')) return 63;
  return -1;
}
static OValue ostd_crypto_base64_encode(OValue value, int url) {
  const unsigned char *data; size_t n; unsigned char *owned=NULL;
  const char *alphabet=url?"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_":"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  if(!ostd_bytes(value,&data,&n,&owned)) return ofail(url?"base64url_encode expects byte-compatible data":"base64_encode expects byte-compatible data");
  if(n>1048576) { free(owned); return ofail(url?"base64url_encode exceeds the 1048576-byte input limit":"base64_encode exceeds the 1048576-byte input limit"); }
  size_t groups=(n+2)/3, maxOut=groups*4;
  char *out=maxOut?(char *)malloc(maxOut+1):NULL;
  if(maxOut&&!out) { free(owned); return ofail("out of memory"); }
  size_t at=0;
  for(size_t i=0;i<n;i+=3) {
    size_t rem=n-i; unsigned int a=data[i], b=rem>1?data[i+1]:0, c=rem>2?data[i+2]:0;
    out[at++]=alphabet[a>>2]; out[at++]=alphabet[((a&3)<<4)|(b>>4)];
    if(rem>1) out[at++]=alphabet[((b&15)<<2)|(c>>6)]; else if(!url) out[at++]='=';
    if(rem>2) out[at++]=alphabet[c&63]; else if(!url) out[at++]='=';
  }
  OValue result=oownedstring(out,at); free(out); free(owned); return result;
}
static OValue ostd_crypto_base64_decode(OValue value, int url) {
  const char *s; size_t n;
  const char *name=url?"base64url_decode":"base64_decode";
  if(!ostd_string(value,&s,&n)) { char message[96]; snprintf(message,sizeof(message),"%s expects a string",name); return ofail(message); }
  if(n>2097152) { char message[128]; snprintf(message,sizeof(message),"%s exceeds the 2097152-byte input limit",name); return ofail(message); }
  size_t useful=n, padding=0;
  if(!url) {
    if(n%4!=0) { char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message); }
    if(n>=1&&s[n-1]=='=') padding++;
    if(n>=2&&s[n-2]=='=') padding++;
    if(padding>2 || (padding&&memchr(s,'=',useful)!=NULL)) { char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message); }
    useful=n-padding;
  } else if(n%4==1 || memchr(s,'=',n)!=NULL) {
    char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message);
  }
  size_t outLen=(useful*3)/4;
  if(outLen>1048576) { char message[128]; snprintf(message,sizeof(message),"%s exceeds the 1048576-byte output limit",name); return ofail(message); }
  unsigned char *out=outLen?(unsigned char *)malloc(outLen):NULL;
  if(outLen&&!out) return ofail("out of memory");
  size_t at=0;
  for(size_t i=0;i<useful;i+=4) {
    size_t rem=useful-i; int a=ostd_crypto_base64_value((unsigned char)s[i],url), b=rem>1?ostd_crypto_base64_value((unsigned char)s[i+1],url):-1, c=rem>2?ostd_crypto_base64_value((unsigned char)s[i+2],url):-1, d=rem>3?ostd_crypto_base64_value((unsigned char)s[i+3],url):-1;
    if(a<0||b<0||(rem>2&&c<0)||(rem>3&&d<0)) { free(out); char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message); }
    if(rem==2 && (b&15)!=0) { free(out); char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message); }
    if(rem==3 && (c&3)!=0) { free(out); char message[96]; snprintf(message,sizeof(message),"%s: invalid encoding",name); return ofail(message); }
    if(at<outLen) out[at++]=(unsigned char)((a<<2)|(b>>4));
    if(rem>2 && at<outLen) out[at++]=(unsigned char)(((b&15)<<4)|(c>>2));
    if(rem>3 && at<outLen) out[at++]=(unsigned char)(((c&3)<<6)|d);
  }
  OValue result=ostd_bytes_list(out,outLen); free(out); return result;
}
static OValue ostd_os_is_path_separator(OValue value) {
  int64_t code;
  if(!ostd_i64(value,&code)||code<0||code>255) return ofail("is_path_separator expects a byte");
#if defined(_WIN32)
  return obool(code=='/'||code=='\\');
#else
  return obool(code=='/');
#endif
}
static int ostd_path(OValue value, const char **data, size_t *len) {
  if(!ostd_string(value,data,len)||*len==0||*len>1048576||memchr(*data,0,*len)!=NULL) return 0;
  return 1;
}
static OValue ostd_os_read_file(OValue path) {
  const char *name; size_t nameLen;
  if(!ostd_path(path,&name,&nameLen)) return ofail("read_file expects a bounded NUL-free path");
  char *copy=(char *)malloc(nameLen+1); if(!copy) return ofail("out of memory");
  memcpy(copy,name,nameLen); copy[nameLen]=0;
  FILE *file=fopen(copy,"rb"); free(copy);
  if(!file) return ofail("os.read_file failed");
  unsigned char *data=NULL; size_t len=0,cap=0;
  for(;;) {
    if(len==1048576) { int extra=fgetc(file); if(extra!=EOF) { fclose(file); free(data); return ofail("read_file exceeds the 1048576-byte limit"); } if(ferror(file)) { fclose(file); free(data); return ofail("os.read_file failed"); } break; }
    if(len==cap) { size_t next=cap?cap*2:4096; if(next>1048576) { fclose(file); free(data); return ofail("read_file exceeds the 1048576-byte limit"); } unsigned char *grown=(unsigned char *)realloc(data,next); if(!grown) { fclose(file); free(data); return ofail("out of memory"); } data=grown; cap=next; }
    size_t n=fread(data+len,1,cap-len,file); len+=n;
    if(n==0) { if(ferror(file)) { fclose(file); free(data); return ofail("os.read_file failed"); } break; }
  }
  fclose(file); OValue out=ostd_bytes_list(data,len); free(data); return out;
}
static OValue ostd_os_write_file(OValue path, OValue bytes, OValue mode) {
  const char *name; size_t nameLen; int64_t modeValue;
  if(!ostd_path(path,&name,&nameLen)||!ostd_i64(mode,&modeValue)||modeValue<0||modeValue>07777) return ofail("write_file expects a bounded path and mode");
  const unsigned char *data; size_t len; unsigned char *owned=NULL;
  if(!ostd_bytes(bytes,&data,&len,&owned)) return ofail("write_file expects byte-compatible data");
  char *copy=(char *)malloc(nameLen+1); if(!copy) { free(owned); return ofail("out of memory"); }
  memcpy(copy,name,nameLen); copy[nameLen]=0; FILE *file=fopen(copy,"wb");
#if !defined(_WIN32)
  if(file) (void)chmod(copy,(mode_t)modeValue);
#endif
  free(copy);
  if(!file) { free(owned); return ofail("os.write_file failed"); }
  size_t written=len?fwrite(data,1,len,file):0; int failed=written!=len; if(fclose(file)!=0) failed=1; free(owned);
  return failed?ofail("os.write_file failed"):obool(1);
}
static OValue ostd_os_getenv(OValue key) {
  const char *name; size_t nameLen;
  if(!ostd_path(key,&name,&nameLen)||memchr(name,'=',nameLen)!=NULL) return ofail("getenv expects a valid variable name");
  char *copy=(char *)malloc(nameLen+1); if(!copy) return ofail("out of memory");
  memcpy(copy,name,nameLen); copy[nameLen]=0; const char *value=getenv(copy); free(copy);
  if(!value) return ooptionNone();
  return ooptionSome(oownedstring(value,strlen(value)));
}
static int ostd_env_compare(const void *left, const void *right) {
  const char *a=*(const char * const *)left, *b=*(const char * const *)right; size_t i=0;
  while(a[i]&&b[i]&&a[i]!='='&&b[i]!='='&&a[i]==b[i]) i++;
  if((a[i]=='='||a[i]==0)&&(b[i]!='='&&b[i]!=0)) return -1;
  if((b[i]=='='||b[i]==0)&&(a[i]!='='&&a[i]!=0)) return 1;
  return (unsigned char)a[i]-(unsigned char)b[i];
}
static OValue ostd_os_env_list(int namesOnly) {
#if defined(_WIN32)
  char **source=_environ;
#else
  extern char **environ; char **source=environ;
#endif
  size_t count=0; if(source) while(source[count]) { if(count>=65536) return ofail("environment exceeds the 65536-entry limit"); count++; }
  char **entries=count?(char **)malloc(count*sizeof(char *)):NULL; if(count&&!entries) return ofail("out of memory");
  for(size_t i=0;i<count;i++) entries[i]=source[i]; qsort(entries,count,sizeof(char *),ostd_env_compare);
  OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) { free(entries); return ofail("out of memory"); }
  size_t outCount=0; for(size_t i=0;i<count;i++) { const char *equals=strchr(entries[i],'='); size_t len=equals?(size_t)(equals-entries[i]):strlen(entries[i]); if(namesOnly) { if(i&&equals&&strchr(entries[i-1],'=')&&strncmp(entries[i-1],entries[i],len)==0&&(size_t)(strchr(entries[i-1],'=')-entries[i-1])==len) continue; items[outCount++]=oownedstring(entries[i],len); } else items[outCount++]=oownedstring(entries[i],strlen(entries[i])); }
  free(entries); OValue out=olist(items,outCount); free(items); return out;
}
static OValue ostd_os_setenv(OValue key, OValue value, int remove) {
  const char *name,*data; size_t nameLen,valueLen;
  if(!ostd_path(key,&name,&nameLen)||memchr(name,'=',nameLen)!=NULL||!ostd_string(value,&data,&valueLen)||memchr(data,0,valueLen)!=NULL) return ofail("environment mutation expects NUL-free names and values");
  char *n=(char *)malloc(nameLen+1), *v=(char *)malloc(valueLen+1); if(!n||!v) { free(n); free(v); return ofail("out of memory"); }
  memcpy(n,name,nameLen); n[nameLen]=0; memcpy(v,data,valueLen); v[valueLen]=0; int result;
#if defined(_WIN32)
  result=remove?_putenv_s(n,""):_putenv_s(n,v);
#else
  result=remove?unsetenv(n):setenv(n,v,1);
#endif
  free(n); free(v); return result==0?obool(1):ofail("environment mutation failed");
}
static OValue ostd_os_identity(const char *member) {
#if defined(_WIN32)
  if(!strcmp(member,"getpagesize")) return oint(4096);
  return ofail("native identity operation is unavailable on this platform");
#else
  if(!strcmp(member,"hostname")) { char name[256]; if(gethostname(name,sizeof(name)-1)!=0) return ofail("os.hostname failed"); name[sizeof(name)-1]=0; return oownedstring(name,strlen(name)); }
  if(!strcmp(member,"getpid")) return oint((int64_t)getpid());
  if(!strcmp(member,"getppid")) return oint((int64_t)getppid());
  if(!strcmp(member,"getuid")) return oint((int64_t)getuid());
  if(!strcmp(member,"geteuid")) return oint((int64_t)geteuid());
  if(!strcmp(member,"getgid")) return oint((int64_t)getgid());
  if(!strcmp(member,"getegid")) return oint((int64_t)getegid());
  if(!strcmp(member,"getpagesize")) { long page=sysconf(_SC_PAGESIZE); return page>0?oint((int64_t)page):ofail("os.getpagesize failed"); }
  return ofail("unknown native identity operation");
#endif
}
static int ostd_time_compare_values(OValue left, OValue right, int *comparison) {
  if(left.kind==O_DURATION&&right.kind==O_DURATION) {
    *comparison=left.as.i<right.as.i?-1:left.as.i>right.as.i?1:0;
    return 1;
  }
  if(left.kind==O_DATETIME&&right.kind==O_DATETIME) {
    if(left.as.datetime.seconds!=right.as.datetime.seconds) {
      *comparison=left.as.datetime.seconds<right.as.datetime.seconds?-1:1;
    } else {
      *comparison=left.as.datetime.nanosecond<right.as.datetime.nanosecond?-1:left.as.datetime.nanosecond>right.as.datetime.nanosecond?1:0;
    }
    return 1;
  }
  return 0;
}
static OValue ostd_time_compare(OValue left, OValue right) {
  int comparison;
  if(!ostd_time_compare_values(left,right,&comparison)) return ofail("time comparison expects two durations or two datetimes");
  return oint(comparison);
}
static OValue ostd_time_predicate(OValue left, OValue right, int relation) {
  int comparison;
  if(!ostd_time_compare_values(left,right,&comparison)) return ofail("time comparison expects two durations or two datetimes");
  return obool(relation<0?comparison<0:relation>0?comparison>0:comparison==0);
}
static uint64_t ostd_time_mod_add(uint64_t left, uint64_t right, uint64_t modulus) {
  return left >= modulus-right ? left-(modulus-right) : left+right;
}
static uint64_t ostd_time_mod_mul(uint64_t left, uint64_t right, uint64_t modulus) {
  uint64_t result=0;
  while(right!=0) {
    if(right&UINT64_C(1)) result=ostd_time_mod_add(result,left,modulus);
    right>>=1;
    if(right!=0) left=ostd_time_mod_add(left,left,modulus);
  }
  return result;
}
static uint64_t ostd_time_zero_remainder(int64_t quantum) {
  uint64_t modulus=(uint64_t)quantum;
  uint64_t seconds=UINT64_C(62135596800)%modulus;
  uint64_t nanoseconds=UINT64_C(1000000000)%modulus;
  return ostd_time_mod_mul(seconds,nanoseconds,modulus);
}
static uint64_t ostd_time_nonnegative_remainder(int64_t value, int64_t quantum) {
  int64_t remainder=value%quantum;
  return remainder<0?(uint64_t)(-(remainder+1))+1:(uint64_t)remainder;
}
static int ostd_time_round_duration(int64_t value, int64_t quantum, int truncate, int64_t *result) {
  if(quantum==0) { *result=value; return 1; }
  int64_t quotient=value/quantum, remainder=value%quantum;
  if(!truncate) {
    uint64_t magnitude=remainder<0?(uint64_t)(-(remainder+1))+1:(uint64_t)remainder;
    uint64_t halfway=(uint64_t)(quantum/2)+(uint64_t)(quantum%2);
    if(magnitude>=halfway) quotient+=value<0?-1:1;
  }
  return !__builtin_mul_overflow(quotient,quantum,result);
}
static int ostd_time_mod_signed(int64_t value, uint64_t modulus, uint64_t *result) {
  if(modulus==0 || modulus>INT64_MAX) return 0;
  int64_t remainder=value%(int64_t)modulus;
  *result=remainder<0?(uint64_t)(-(remainder+1))+1:(uint64_t)remainder;
  return 1;
}
static int ostd_time_shift_datetime(ODatetime value, int64_t delta, ODatetime *result) {
  int64_t secondsDelta=delta/1000000000, nanosecondDelta=delta%1000000000, seconds;
  if(__builtin_add_overflow(value.seconds,secondsDelta,&seconds)) return 0;
  int64_t nanosecond=(int64_t)value.nanosecond+nanosecondDelta;
  if(nanosecond<0) { if(seconds==INT64_MIN) return 0; seconds--; nanosecond+=1000000000; }
  if(nanosecond>=1000000000) { if(seconds==INT64_MAX) return 0; seconds++; nanosecond-=1000000000; }
  result->seconds=seconds; result->nanosecond=(int32_t)nanosecond;
  return 1;
}
static int ostd_time_round_datetime(ODatetime value, int64_t quantum, int truncate, ODatetime *result) {
  if(quantum==0) { *result=value; return 1; }
  uint64_t modulus=(uint64_t)quantum, secondsRemainder, anchorRemainder;
  if(!ostd_time_mod_signed(value.seconds,modulus,&secondsRemainder)||!ostd_time_mod_signed(INT64_C(62135596800),modulus,&anchorRemainder)) return 0;
  uint64_t nanosPerSecond=UINT64_C(1000000000)%modulus;
  uint64_t remainder=ostd_time_mod_add(ostd_time_mod_mul(secondsRemainder,nanosPerSecond,modulus),ostd_time_mod_mul(anchorRemainder,nanosPerSecond,modulus),modulus);
  remainder=ostd_time_mod_add(remainder,(uint64_t)value.nanosecond,modulus);
  uint64_t delta;
  if(truncate) delta=remainder;
  else {
    uint64_t halfway=(uint64_t)(quantum/2)+(uint64_t)(quantum%2);
    delta=remainder<halfway?remainder:modulus-remainder;
  }
  if(delta>INT64_MAX) return 0;
  return ostd_time_shift_datetime(value,truncate?-(int64_t)delta:(remainder<((uint64_t)(quantum/2)+(uint64_t)(quantum%2))?(int64_t)-delta:(int64_t)delta),result);
}
static OValue ostd_time_round(OValue value, OValue quantumValue, int truncate) {
  int64_t quantum,result;
  if(quantumValue.kind!=O_DURATION) return ofail("time rounding expects a duration quantum");
  quantum=quantumValue.as.i;
  if(quantum<0) return ofail(truncate?"truncate quantum must not be negative":"round quantum must not be negative");
  if(value.kind==O_DURATION) {
    if(!ostd_time_round_duration(value.as.i,quantum,truncate,&result)) return ofail("duration rounding overflow");
    return (OValue){O_DURATION,0,{.i=result}};
  }
  if(value.kind==O_DATETIME) {
    ODatetime rounded;
    if(!ostd_time_round_datetime(value.as.datetime,quantum,truncate,&rounded)) return ofail("datetime rounding result is outside native range");
    return (OValue){O_DATETIME,0,{.datetime=rounded}};
  }
  return ofail(truncate?"truncate expects duration or datetime":"round expects duration or datetime");
}
static int ostd_time_civil(OValue date, int *year, unsigned *month, unsigned *day, int *hour, int *minute, int *second, int *nanosecond) {
  if(date.kind!=O_DATETIME) return 0;
  int64_t seconds=date.as.datetime.seconds, nanos=date.as.datetime.nanosecond;
  int64_t days=seconds/86400, tod=seconds%86400;
  if(tod<0) { days--; tod+=86400; }
  ocivil_from_days(days,year,month,day);
  if(hour) *hour=(int)(tod/3600);
  if(minute) *minute=(int)(tod/60%60);
  if(second) *second=(int)(tod%60);
  if(nanosecond) *nanosecond=(int)nanos;
  return *year>=1&&*year<=9999;
}
static OValue ostd_time_component(OValue date, const char *name, int component) {
  int year,hour,minute,second,nanosecond; unsigned month,day;
  static const char *arity[] = {"year expects 1 datetime", "month expects 1 datetime", "day expects 1 datetime", "year_day expects 1 datetime", "weekday expects 1 datetime", "hour expects 1 datetime", "minute expects 1 datetime", "second expects 1 datetime", "nanosecond expects 1 datetime"};
  if(date.kind!=O_DATETIME) return ofail(component>=0&&component<9?arity[component]:"datetime component expects datetime");
  if(!ostd_time_civil(date,&year,&month,&day,&hour,&minute,&second,&nanosecond)) return ofail("datetime is outside the supported civil range");
  if(component==0) return ouint((uint64_t)year);
  if(component==1) return ouint((uint64_t)month);
  if(component==2) return ouint((uint64_t)day);
  if(component==3) return ouint((uint64_t)(odays_from_civil(year,(unsigned)month,day)-odays_from_civil(year,1,1)+1));
  if(component==5) return ouint((uint64_t)hour);
  if(component==6) return ouint((uint64_t)minute);
  if(component==7) return ouint((uint64_t)second);
  if(component==8) return ouint((uint64_t)nanosecond);
  int weekday=(int)((odays_from_civil(year,(unsigned)month,day)+4)%7); if(weekday<0) weekday+=7;
  static const char *names[] = {"Sunday","Monday","Tuesday","Wednesday","Thursday","Friday","Saturday"};
  if(component==4) return ostring(names[weekday]);
  (void)name;
  return ofail("unsupported datetime component");
}
static OValue ostd_time_utc(OValue value) {
  if(value.kind!=O_DATETIME) return ofail("utc expects 1 datetime");
  return value;
}

static OValue ostd_dispatch(const char *module, const char *member, const OValue *args, size_t argc) {
  if(!strcmp(module,"std/text") && !strcmp(member,"contains_any")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("contains_any expects two strings"); OValue found=ostd_text_index(args[0],args[1],0,1); return found.kind==O_ERROR?found:obool(found.kind==O_OPTION&&found.fbits!=0); }
  if(!strcmp(module,"std/text")) {
    if(!strcmp(member,"trim")||!strcmp(member,"trim_space")) { if(argc!=1) return ofail("trim expects 1 argument"); return ostd_text_trim(args[0],1,1); }
    if(!strcmp(member,"upper")) { if(argc!=1) return ofail("upper expects 1 argument"); return ostd_text_case(args[0],1); }
    if(!strcmp(member,"lower")) { if(argc!=1) return ofail("lower expects 1 argument"); return ostd_text_case(args[0],0); }
    if(!strcmp(member,"fields")) { if(argc!=1) return ofail("fields expects 1 argument"); return ostd_text_fields(args[0]); }
    if(!strcmp(member,"join")) { if(argc!=2) return ofail("join expects 2 arguments"); return ostd_text_join(args[0],args[1]); }
    if(!strcmp(member,"cat")) return ocat(args,argc);
    if(!strcmp(member,"slug")) return ostd_text_slug(args[0]);
    if(!strcmp(member,"replace")) { if(argc!=3) return ofail("replace expects 3 arguments"); return ostd_text_replace(args[0],args[1],args[2],onull()); }
    if(!strcmp(member,"replace_n")) { if(argc!=4) return ofail("replace_n expects 4 arguments"); return ostd_text_replace(args[0],args[1],args[2],args[3]); }
    if(!strcmp(member,"repeat")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)||n<0) return ofail("repeat expects string and non-negative count"); if(args[0].kind!=O_STRING||n>1048576/(args[0].as.s.len?args[0].as.s.len:1)) return ofail("repeat result is too large"); size_t len=args[0].as.s.len*(size_t)n; char *out=len?(char *)malloc(len+1):NULL; if(len&&!out) return ofail("out of memory"); for(int64_t i=0;i<n;i++) memcpy(out+i*args[0].as.s.len,args[0].as.s.data,args[0].as.s.len); OValue result=oownedstring(out,len); free(out); return result; }
    if(!strcmp(member,"split")||!strcmp(member,"split_after")) { if(argc!=2) return ofail("split expects 2 arguments"); return ostd_text_split(args[0],args[1],!strcmp(member,"split_after"),-1); }
    if(!strcmp(member,"split_n")||!strcmp(member,"split_after_n")) { int64_t n; if(argc!=3||!ostd_i64(args[2],&n)||n<0) return ofail("split limit must be non-negative"); return ostd_text_split(args[0],args[1],!strcmp(member,"split_after_n"),n); }
    if(!strcmp(member,"chunks")) { if(argc!=2) return ofail("chunks expects 2 arguments"); return ostd_text_chunks(args[0],args[1]); }
    if(!strcmp(member,"compare")||!strcmp(member,"contains")||!strcmp(member,"has_prefix")||!strcmp(member,"has_suffix")||!strcmp(member,"equal_fold")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("text comparison expects two strings"); if(!strcmp(member,"compare")) { size_t common=args[0].as.s.len<args[1].as.s.len?args[0].as.s.len:args[1].as.s.len; int cmp=memcmp(args[0].as.s.data,args[1].as.s.data,common); if(cmp==0) cmp=args[0].as.s.len==args[1].as.s.len?0:args[0].as.s.len<args[1].as.s.len?-1:1; return oint(cmp<0?-1:cmp>0?1:0); } if(!strcmp(member,"contains")) return obool(args[1].as.s.len==0||strstr(args[0].as.s.data,args[1].as.s.data)!=NULL); if(!strcmp(member,"has_prefix")) return obool(args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data,args[1].as.s.data,args[1].as.s.len)); if(!strcmp(member,"has_suffix")) return obool(args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data+args[0].as.s.len-args[1].as.s.len,args[1].as.s.data,args[1].as.s.len)); return ostd_text_equal_fold(args[0],args[1]); }
    if(!strcmp(member,"count")) { if(argc!=2) return ofail("count expects 2 arguments"); return ostd_text_count(args[0],args[1]); }
    if(!strcmp(member,"index")||!strcmp(member,"last_index")||!strcmp(member,"index_any")||!strcmp(member,"last_index_any")) { if(argc!=2) return ofail("index expects 2 arguments"); return ostd_text_index(args[0],args[1],strstr(member,"last")!=NULL,strstr(member,"any")!=NULL); }
    if(!strcmp(member,"cut")||!strcmp(member,"cut_prefix")||!strcmp(member,"cut_suffix")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("cut expects strings"); const char *p=NULL; if(!strcmp(member,"cut_suffix")) { if(args[1].as.s.len<=args[0].as.s.len) { const char *q=args[0].as.s.data+args[0].as.s.len-args[1].as.s.len; if(!memcmp(q,args[1].as.s.data,args[1].as.s.len)) p=q; } if(!p) return ooptionNone(); return ooptionSome(ostringview(args[0].as.s.data,p-args[0].as.s.data)); } p=strstr(args[0].as.s.data,args[1].as.s.data); if(!p) return ooptionNone(); if(!strcmp(member,"cut_prefix")&&p!=args[0].as.s.data) return ooptionNone(); if(!strcmp(member,"cut_prefix")) return ooptionSome(ostringview(p+args[1].as.s.len,args[0].as.s.len-args[1].as.s.len)); OValue pair[2]={ostringview(args[0].as.s.data,p-args[0].as.s.data),ostringview(p+args[1].as.s.len,args[0].as.s.len-(size_t)(p-args[0].as.s.data)-args[1].as.s.len)}; return ooptionSome(olist(pair,2)); }
    if(!strcmp(member,"trim_left")||!strcmp(member,"trim_right")) { if(argc!=2) return ofail("trim character set expects strings"); return ostd_text_trim_set(args[0],args[1],!strcmp(member,"trim_left")); }
    if(!strcmp(member,"trim_prefix")||!strcmp(member,"trim_suffix")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("trim prefix expects strings"); if(!strcmp(member,"trim_prefix")&&args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data,args[1].as.s.data,args[1].as.s.len)) return oownedstring(args[0].as.s.data+args[1].as.s.len,args[0].as.s.len-args[1].as.s.len); if(!strcmp(member,"trim_suffix")&&args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data+args[0].as.s.len-args[1].as.s.len,args[1].as.s.data,args[1].as.s.len)) return oownedstring(args[0].as.s.data,args[0].as.s.len-args[1].as.s.len); return oownedstring(args[0].as.s.data,args[0].as.s.len); }
  }
  if(!strcmp(module,"std/list")) { if(!strcmp(member,"range")) { if(argc!=1) return ofail("range expects 1 argument"); return ostd_list_range(args[0]); } if(!strcmp(member,"len")) { if(argc!=1) return ofail("len expects 1 argument"); if(args[0].kind==O_STRING) { size_t n=ostd_runes(args[0].as.s.data,args[0].as.s.len); return n==SIZE_MAX?ofail("len expects valid UTF-8"):ofloat((double)n); } if(args[0].kind==O_LIST) return ofloat((double)args[0].as.list.len); if(args[0].kind==O_TENSOR) return ofloat((double)args[0].as.tensor.len); return ofail("len expects a string, list, or tensor"); } if(!strcmp(member,"sum")||!strcmp(member,"product")) return argc==1?(!strcmp(member,"product")?ostd_list_product(args[0]):ostd_list_sum(args[0])):ofail("list reduction expects 1 argument"); if(!strcmp(member,"take")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)) return ofail("take expects a value and integer count"); return ostd_list_take(args[0],n); } if(!strcmp(member,"reverse")) return argc==1?ostd_list_reverse(args[0]):ofail("reverse expects 1 argument"); if(!strcmp(member,"flatten")) return argc==2?ostd_list_flatten(args[0],args[1]):ofail("flatten expects a list and depth"); if(!strcmp(member,"at")) return argc==2?ostd_list_at(args[0],args[1]):ofail("at expects 2 arguments"); if(!strcmp(member,"partition")) return argc==2?ostd_list_partition(args[0],args[1]):ofail("partition expects list and index"); if(!strcmp(member,"index_of")||!strcmp(member,"last_index_of")||!strcmp(member,"contains")||!strcmp(member,"count")) { if(argc!=2) return ofail("list search expects 2 arguments"); OValue result=ostd_list_index(args[0],args[1],!strcmp(member,"last_index_of"),!strcmp(member,"count")); if(!strcmp(member,"contains")) return result.kind==O_ERROR?result:obool(result.kind==O_INT&&result.as.i>=0); return result; } if(!strcmp(member,"sort")) return argc==1?ostd_list_sort(args[0]):ofail("sort expects 1 argument"); if(!strcmp(member,"unique")) return argc==1?ostd_list_unique(args[0]):ofail("unique expects 1 argument"); if(!strcmp(member,"slice")) { int64_t a,b; if(argc!=3||!ostd_i64(args[1],&a)||!ostd_i64(args[2],&b)) return ofail("slice expects list and integer bounds"); return ostd_list_shape(args[0],0,a,b); } if(!strcmp(member,"chunks")||!strcmp(member,"windows")) { int64_t a,b=0; if((!strcmp(member,"chunks")&&argc!=2)||(!strcmp(member,"windows")&&argc!=3)||!ostd_i64(args[1],&a)||(argc==3&&!ostd_i64(args[2],&b))) return ofail("list shape operation expects integer bounds"); return ostd_list_shape(args[0],!strcmp(member,"chunks")?1:2,a,b); } if(!strcmp(member,"zip")) return argc==2?ostd_list_zip(args[0],args[1]):ofail("zip expects 2 arguments"); if(!strcmp(member,"concat")) { size_t total=0; for(size_t i=0;i<argc;i++) { if(args[i].kind!=O_LIST||total>100000||args[i].as.list.len>100000-total||args[i].as.list.len>SIZE_MAX-total) return ofail("concat expects bounded lists"); total+=args[i].as.list.len; } OValue *items=total?(OValue *)malloc(total*sizeof(OValue)):NULL; if(total&&!items)return ofail("out of memory"); size_t at=0; for(size_t i=0;i<argc;i++){memcpy(items+at,args[i].as.list.items,args[i].as.list.len*sizeof(OValue));at+=args[i].as.list.len;} OValue out=olist(items,total);free(items);return out; } if(!strcmp(member,"repeat")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)||n<0||n>100000) return ofail("repeat expects bounded item count"); OValue *items=n?(OValue *)malloc((size_t)n*sizeof(OValue)):NULL;if(n&&!items)return ofail("out of memory");for(int64_t i=0;i<n;i++)items[i]=args[0];OValue out=olist(items,(size_t)n);free(items);return out; } if(!strcmp(member,"empty")) return argc==0?olist(NULL,0):ofail("empty expects no arguments"); if(!strcmp(member,"all")||!strcmp(member,"any")||!strcmp(member,"none")) { if(argc!=1) return ofail("list predicate expects a list"); if(!ostd_list_bounded(args[0],"list predicate expects a list")) return onull(); int wantAny=!strcmp(member,"any"),wantNone=!strcmp(member,"none"); for(size_t i=0;i<args[0].as.list.len;i++) { int truth=otruthy(args[0].as.list.items[i]); if((wantAny&&truth)||(!wantAny&&!wantNone&&!truth)||(wantNone&&truth)) return obool(wantAny?1:0); } return obool(!wantAny); } if(!strcmp(member,"enumerate")) { if(argc!=1) return ofail("enumerate expects 1 argument"); if(!ostd_list_bounded(args[0],"enumerate expects 1 argument")) return onull(); OValue *items=args[0].as.list.len?(OValue *)malloc(args[0].as.list.len*sizeof(OValue)):NULL;if(args[0].as.list.len&&!items)return ofail("out of memory");for(size_t i=0;i<args[0].as.list.len;i++){OValue p[2]={ouint(i),args[0].as.list.items[i]};items[i]=olist(p,2);if(items[i].kind==O_LIST)items[i].fbits=1;}OValue out=olist(items,args[0].as.list.len);free(items);return out; } }
  if(!strcmp(module,"std/signal")) { if(!strcmp(member,"rectangular")||!strcmp(member,"hann")||!strcmp(member,"hamming")||!strcmp(member,"blackman")||!strcmp(member,"bartlett")) { if(argc!=1) return ofail("signal window expects 1 argument"); return ostd_signal_window(args[0],!strcmp(member,"rectangular")?0:!strcmp(member,"hann")?1:!strcmp(member,"hamming")?2:!strcmp(member,"blackman")?3:4); } if(!strcmp(member,"convolve")||!strcmp(member,"correlate")) { if(argc!=2) return ofail("signal operation expects 2 arguments"); return ostd_signal_conv(args[0],args[1],!strcmp(member,"correlate")); } if(!strcmp(member,"filter_fir")) { if(argc!=2) return ofail("signal operation expects 2 arguments"); return ostd_signal_fir(args[0],args[1]); } if(!strcmp(member,"resample")) return argc==2?ostd_signal_resample(args[0],args[1]):ofail("signal.resample expects signal and output length"); if(!strcmp(member,"mean")||!strcmp(member,"energy")||!strcmp(member,"variance")||!strcmp(member,"rms")) { if(argc!=1) return ofail("signal statistic expects 1 argument"); return ostd_signal_stats(args[0],!strcmp(member,"mean")?0:!strcmp(member,"energy")?1:!strcmp(member,"variance")?2:3); } if(!strcmp(member,"fft")||!strcmp(member,"ifft")) return argc==1?ostd_signal_fft(args[0],!strcmp(member,"ifft")):ofail("fft expects 1 argument"); if(!strcmp(member,"normalize")) return argc==2?ostd_signal_normalize(args[0],args[1]):ofail("normalize expects signal and mode"); }
  if(!strcmp(module,"std/crypto")) { if(!strcmp(member,"hash")) { OValue rendered=ocat(args,argc); if(rendered.kind==O_ERROR)return rendered; return ostd_crypto_digest(rendered,ostring("sha256"),0); } if(!strcmp(member,"digest")||!strcmp(member,"digest_bytes")) return argc==2?ostd_crypto_digest(args[0],args[1],!strcmp(member,"digest_bytes")):ofail("digest expects data and algorithm"); if(!strcmp(member,"hmac")) return argc==3?ostd_crypto_hmac(args[0],args[1],args[2]):ofail("crypto.hmac expects key, data, and algorithm"); if(!strcmp(member,"hex_encode")||!strcmp(member,"hex_decode")) return argc==1?ostd_crypto_hex(args[0],!strcmp(member,"hex_decode")):ofail("hex codec expects 1 argument"); if(!strcmp(member,"compare_digest")) return argc==2?ostd_crypto_compare_digest(args[0],args[1]):ofail("compare_digest expects 2 arguments"); if(!strcmp(member,"base64_encode")||!strcmp(member,"base64url_encode")) return argc==1?ostd_crypto_base64_encode(args[0],!strcmp(member,"base64url_encode")):ofail("base64 encode expects 1 argument"); if(!strcmp(member,"base64_decode")||!strcmp(member,"base64url_decode")) return argc==1?ostd_crypto_base64_decode(args[0],!strcmp(member,"base64url_decode")):ofail("base64 decode expects 1 argument"); if(!strcmp(member,"algorithm_supported")) { if(argc!=1||args[0].kind!=O_STRING)return ofail("algorithm_supported expects a string");return obool(!strcmp(args[0].as.s.data,"sha256")||!strcmp(args[0].as.s.data,"sha512")||!strcmp(args[0].as.s.data,"sha512_256")); } }
  if(!strcmp(module,"std/time")) { if(!strcmp(member,"parse_duration")) return argc==1?ostd_time_parse_duration(args[0]):ofail("parse_duration expects 1 argument"); if(!strcmp(member,"duration_string")) return argc==1?ostd_time_duration_string(args[0]):ofail("duration_string expects 1 argument"); if(!strcmp(member,"add")) return argc==2?ostd_time_add(args[0],args[1]):ofail("add expects 2 arguments"); if(!strcmp(member,"sub")) return argc==2?ostd_time_sub(args[0],args[1]):ofail("sub expects 2 arguments"); if(!strcmp(member,"round")) return argc==2?ostd_time_round(args[0],args[1],0):ofail("round expects 2 arguments"); if(!strcmp(member,"truncate")) return argc==2?ostd_time_round(args[0],args[1],1):ofail("truncate expects 2 arguments"); if(!strcmp(member,"format")) return argc==2?ostd_time_format(args[0],args[1]):ofail("format expects 2 arguments"); if(!strcmp(member,"date")) return (argc==7||argc==8)?ostd_time_date(args,(size_t)argc):ofail("date expects 7 fields and an optional location"); if(!strcmp(member,"add_date")) return argc==4?ostd_time_add_date(args[0],args[1],args[2],args[3]):ofail("add_date expects 4 arguments"); if(!strcmp(member,"compare")) return argc==2?ostd_time_compare(args[0],args[1]):ofail("compare expects 2 arguments"); if(!strcmp(member,"equal")) return argc==2?ostd_time_predicate(args[0],args[1],0):ofail("equal expects 2 arguments"); if(!strcmp(member,"before")) return argc==2?ostd_time_predicate(args[0],args[1],-1):ofail("before expects 2 arguments"); if(!strcmp(member,"after")) return argc==2?ostd_time_predicate(args[0],args[1],1):ofail("after expects 2 arguments"); if(!strcmp(member,"year")) return argc==1?ostd_time_component(args[0],"year",0):ofail("year expects 1 datetime"); if(!strcmp(member,"month")) return argc==1?ostd_time_component(args[0],"month",1):ofail("month expects 1 datetime"); if(!strcmp(member,"day")) return argc==1?ostd_time_component(args[0],"day",2):ofail("day expects 1 datetime"); if(!strcmp(member,"year_day")) return argc==1?ostd_time_component(args[0],"year_day",3):ofail("year_day expects 1 datetime"); if(!strcmp(member,"weekday")) return argc==1?ostd_time_component(args[0],"weekday",4):ofail("weekday expects 1 datetime"); }
  if(!strcmp(module,"std/time")) {
    if(!strcmp(member,"hour")) return argc==1?ostd_time_component(args[0],"hour",5):ofail("hour expects 1 datetime");
    if(!strcmp(member,"minute")) return argc==1?ostd_time_component(args[0],"minute",6):ofail("minute expects 1 datetime");
    if(!strcmp(member,"second")) return argc==1?ostd_time_component(args[0],"second",7):ofail("second expects 1 datetime");
    if(!strcmp(member,"nanosecond")) return argc==1?ostd_time_component(args[0],"nanosecond",8):ofail("nanosecond expects 1 datetime");
    if(!strcmp(member,"utc")) return argc==1?ostd_time_utc(args[0]):ofail("utc expects 1 datetime");
  }
  if(!strcmp(module,"std/os")) {
    if(!strcmp(member,"is_path_separator")) return argc==1?ostd_os_is_path_separator(args[0]):ofail("is_path_separator expects 1 argument");
    if(!strcmp(member,"hostname")||!strcmp(member,"getpid")||!strcmp(member,"getppid")||!strcmp(member,"getuid")||!strcmp(member,"geteuid")||!strcmp(member,"getgid")||!strcmp(member,"getegid")||!strcmp(member,"getpagesize")) {
#if defined(OSIRIS_CAP_OS_SYSTEM_IDENTITY)
      return argc==0?ostd_os_identity(member):ofail("identity operation expects no arguments");
#else
      return ofail("native capability os.system_identity is not granted");
#endif
    }
    if(!strcmp(member,"read_file")) {
#if defined(OSIRIS_CAP_OS_FILE_READ)
      return argc==1?ostd_os_read_file(args[0]):ofail("read_file expects 1 argument");
#else
      return ofail("native capability os.file_read is not granted");
#endif
    }
    if(!strcmp(member,"write_file")) {
#if defined(OSIRIS_CAP_OS_FILE_WRITE)
      return argc==3?ostd_os_write_file(args[0],args[1],args[2]):ofail("write_file expects 3 arguments");
#else
      return ofail("native capability os.file_write is not granted");
#endif
    }
    if(!strcmp(member,"getenv")) {
#if defined(OSIRIS_CAP_OS_ENVIRONMENT)
      return argc==1?ostd_os_getenv(args[0]):ofail("getenv expects 1 argument");
#else
      return ofail("native capability os.environment is not granted");
#endif
    }
    if(!strcmp(member,"env_names")||!strcmp(member,"environ")) {
#if defined(OSIRIS_CAP_OS_ENVIRONMENT)
      return argc==0?ostd_os_env_list(!strcmp(member,"env_names")):ofail("environment listing expects no arguments");
#else
      return ofail("native capability os.environment is not granted");
#endif
    }
    if(!strcmp(member,"setenv")||!strcmp(member,"unsetenv")) {
#if defined(OSIRIS_CAP_OS_ENVIRONMENT_MUTATE)
      if(!strcmp(member,"setenv")) return argc==2?ostd_os_setenv(args[0],args[1],0):ofail("setenv expects name and value");
      return argc==1?ostd_os_setenv(args[0],ostring(""),1):ofail("unsetenv expects a name");
#else
      return ofail("native capability os.environment_mutate is not granted");
#endif
    }
    return ofail("native std/os host operation requires an explicit capability and is not available in this artifact");
  }
  return ofail("native standard-library member is not implemented");
}
`
