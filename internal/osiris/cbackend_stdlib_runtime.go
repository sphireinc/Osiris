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
static int ostd_string(OValue value, const char **data, size_t *len) { if(value.kind!=O_STRING) return 0; *data=value.as.s.data; *len=value.as.s.len; return 1; }
static int ostd_bytes(OValue value, const unsigned char **data, size_t *len, unsigned char **owned) {
  *owned=NULL;
  if(value.kind==O_STRING) { *data=(const unsigned char *)value.as.s.data; *len=value.as.s.len; return 1; }
  if(value.kind!=O_LIST) return 0;
  if(value.as.list.len>1048576) return 0;
  *owned=value.as.list.len?(unsigned char *)malloc(value.as.list.len):NULL;
  if(value.as.list.len&&!*owned) return 0;
  for(size_t i=0;i<value.as.list.len;i++) { if(value.as.list.items[i].kind!=O_UINT||value.as.list.items[i].as.u>255) { free(*owned); *owned=NULL; return 0; } (*owned)[i]=(unsigned char)value.as.list.items[i].as.u; }
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
static OValue ostd_text_case(OValue value, int upper) {
  const char *data; size_t len,at=0,n=0; uint32_t r; if(!ostd_text_arg(value,&data,&len,"case operation expects a string")) return onull();
  char *out=(char *)malloc(len*2+1); if(len&& !out) return ofail("out of memory");
  while(at<len) { if(!outf8decode(data,len,&at,&r)) { free(out); return ofail("text expects valid UTF-8"); } if(upper&&r==0xdf) { out[n++]='S'; out[n++]='S'; } else n+=outf8encode(out+n,upper?ounicodeupper(r):ounicodelower(r)); }
  OValue result=oownedstring(out,n); free(out); return result;
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
static OValue ostd_text_chunks(OValue text, int64_t width) {
  const char *s; size_t sn; if(!ostd_string(text,&s,&sn)||width<=0) return ofail("chunks expects text and positive size");
  size_t *starts=NULL, count=0, cap=0, at=0; uint32_t r;
  while(at<sn) { if(count==cap) { size_t nc=cap?cap*2:8; size_t *ns=(size_t *)realloc(starts,nc*sizeof(size_t)); if(!ns) { free(starts); return ofail("out of memory"); } starts=ns; cap=nc; } starts[count++]=at; for(int64_t i=0;i<width&&at<sn;i++) { if(!outf8decode(s,sn,&at,&r)) { free(starts); return ofail("chunks expects valid UTF-8"); } } }
  OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) { free(starts); return ofail("out of memory"); } for(size_t i=0;i<count;i++) { size_t end=i+1<count?starts[i+1]:sn; items[i]=ostringview(s+starts[i],end-starts[i]); } OValue out=olist(items,count); free(items); free(starts); return out;
}
static OValue ostd_text_join(OValue separator, OValue values) { if(separator.kind!=O_STRING||values.kind!=O_LIST) return ofail("join expects a string separator and list"); size_t len=0; for(size_t i=0;i<values.as.list.len;i++) { size_t part=ocatpart(NULL,0,values.as.list.items[i]); if(part>SIZE_MAX-len||(i&&separator.as.s.len>SIZE_MAX-len-part)) return ofail("join result is too large"); len+=part+(i?separator.as.s.len:0); } char *out=(char *)malloc(len+1); if(len&&!out) return ofail("out of memory"); size_t at=0; for(size_t i=0;i<values.as.list.len;i++) { if(i) { memcpy(out+at,separator.as.s.data,separator.as.s.len); at+=separator.as.s.len; } at+=ocatpart(out+at,len-at,values.as.list.items[i]); } OValue result=oownedstring(out,len); free(out); return result; }
static OValue ostd_text_fields(OValue value) { const char *s; size_t sn; if(!ostd_string(value,&s,&sn)) return ofail("fields expects a string"); OValue *items=NULL; size_t count=0,cap=0,at=0,start=0; uint32_t r; while(at<sn) { size_t before=at; if(!outf8decode(s,sn,&at,&r)) { free(items); return ofail("fields expects valid UTF-8"); } if(ounicodespace(r)) { if(start<before) { if(count==cap) { cap=cap?cap*2:8; items=(OValue *)realloc(items,cap*sizeof(OValue)); if(!items) return ofail("out of memory"); } items[count++]=ostringview(s+start,before-start); } start=at; } } if(start<sn) { if(count==cap) { cap=cap?cap*2:8; items=(OValue *)realloc(items,cap*sizeof(OValue)); if(!items) return ofail("out of memory"); } items[count++]=ostringview(s+start,sn-start); } OValue out=olist(items,count); free(items); return out; }
static OValue ostd_text_index(OValue text, OValue needle, int last, int any) { const char *s,*n; size_t sn,nn; if(!ostd_string(text,&s,&sn)||!ostd_string(needle,&n,&nn)) return ofail("text search expects strings"); size_t p=SIZE_MAX; if(any) { size_t at=0; uint32_t a,b; while(at<sn) { size_t before=at; if(!outf8decode(s,sn,&at,&a)) return ofail("text expects valid UTF-8"); size_t scan=0; while(scan<nn) { if(!outf8decode(n,nn,&scan,&b)) return ofail("text expects valid UTF-8"); if(a==b) { p=before; if(!last) goto found; break; } } } } else { if(last) { const char *f=s; while((f=strstr(f,n))!=NULL) { p=(size_t)(f-s); f+=nn?nn:1; } } else { const char *f=strstr(s,n); if(f) p=(size_t)(f-s); } } found: if(p==SIZE_MAX) return ooptionNone(); size_t runes=0,at=0; uint32_t r; while(at<p) { if(!outf8decode(s,sn,&at,&r)) return ofail("text expects valid UTF-8"); runes++; } return ooptionSome(ouint(runes)); }
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
static OValue ostd_list_at(OValue value, OValue index) { int64_t i; if(value.kind!=O_LIST||!ostd_i64(index,&i)||i<0) return ofail("at expects a list and non-negative index"); return (size_t)i<value.as.list.len?value.as.list.items[i]:ofail("list index out of range"); }
static OValue ostd_list_index(OValue value, OValue target, int last, int wantCount) { if(value.kind!=O_LIST) return ofail("index expects a list"); int64_t found=-1,count=0; for(size_t i=0;i<value.as.list.len;i++) { OValue eq=oequal(value.as.list.items[i],target); if(eq.kind==O_ERROR) return eq; if(eq.kind==O_BOOL&&eq.as.b) { count++; found=(int64_t)i; if(!last&&!wantCount) break; } } return wantCount?ouint((uint64_t)count):oint(found); }
static OValue ostd_list_sort(OValue value) { if(value.kind!=O_LIST) return ofail("sort expects a list"); OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL; if(value.as.list.len&&!items) return ofail("out of memory"); if(value.as.list.len) memcpy(items,value.as.list.items,value.as.list.len*sizeof(OValue)); for(size_t i=1;i<value.as.list.len;i++) { OValue item=items[i]; size_t j=i; while(j>0) { OValue less=obin("<",item,items[j-1]); if(less.kind==O_ERROR) { free(items); return less; } if(!less.as.b) break; items[j]=items[j-1]; j--; } items[j]=item; } OValue out=olist(items,value.as.list.len); free(items); return out; }
static OValue ostd_list_unique(OValue value) { if(value.kind!=O_LIST) return ofail("unique expects a list"); OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL; if(value.as.list.len&&!items) return ofail("out of memory"); size_t n=0; for(size_t i=0;i<value.as.list.len;i++) { int seen=0; for(size_t j=0;j<n;j++) { OValue eq=oequal(items[j],value.as.list.items[i]); if(eq.kind==O_ERROR) { free(items); return eq; } if(eq.as.b) { seen=1; break; } } if(!seen) items[n++]=value.as.list.items[i]; } OValue out=olist(items,n); free(items); return out; }
static OValue ostd_list_zip(OValue left, OValue right) { if(left.kind!=O_LIST||right.kind!=O_LIST) return ofail("zip expects two lists"); size_t n=left.as.list.len<right.as.list.len?left.as.list.len:right.as.list.len; OValue *items=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!items) return ofail("out of memory"); for(size_t i=0;i<n;i++) { OValue pair[2]={left.as.list.items[i],right.as.list.items[i]}; items[i]=olist(pair,2); } OValue out=olist(items,n); free(items); return out; }
static OValue ostd_list_shape(OValue value, int mode, int64_t a, int64_t b) { if(value.kind!=O_LIST||a<0||b<0||a>b) return ofail("list shape arguments are invalid"); size_t start=(size_t)a,end=(size_t)b; if(start>value.as.list.len) start=value.as.list.len; if(end>value.as.list.len) end=value.as.list.len; if(mode==0) return olist(value.as.list.items+start,end-start); size_t width=(size_t)a; if(mode==1&&width==0) return ofail("chunks width must be positive"); if(mode==2&&width==0) return ofail("window width must be positive"); size_t step=(size_t)b; if(mode==2&&step==0) return ofail("window step must be positive"); size_t count=mode==1?(value.as.list.len+width-1)/width:(value.as.list.len>=width?(value.as.list.len-width)/step+1:0); OValue *out=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!out) return ofail("out of memory"); for(size_t i=0;i<count;i++) { size_t p=mode==1?i*width:i*step; size_t n=mode==1?(value.as.list.len-p<width?value.as.list.len-p:width):width; out[i]=olist(value.as.list.items+p,n); } OValue result=olist(out,count); free(out); return result; }
static OValue ostd_signal_window(OValue value, int kind) { int64_t n; if(!ostd_i64(value,&n)||n<0||n>65536) return ofail("signal window length is out of range"); size_t count=(size_t)n; double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); for(size_t i=0;i<count;i++) { double x=count>1?(double)i/(double)(count-1):0; data[i]=kind==0?1.0:kind==1?0.5-0.5*ogocos(2*3.14159265358979323846*x):kind==2?0.54-0.46*ogocos(2*3.14159265358979323846*x):1.0-fabs(2*x-1.0); } size_t shape[1]={count}; return otensordata(data,count,shape,1); }
static int ostd_signal_data(OValue value, double **out, size_t *n) { if(value.kind==O_TENSOR&&value.as.tensor.rank==1) { *n=value.as.tensor.len; *out=(double *)malloc(*n*sizeof(double)); if(*n&&!*out) return 0; if(*n) memcpy(*out,value.as.tensor.data,*n*sizeof(double)); return 1; } if(value.kind!=O_LIST) return 0; *n=value.as.list.len; *out=*n?(double *)malloc(*n*sizeof(double)):NULL; if(*n&&!*out) return 0; for(size_t i=0;i<*n;i++) if(!onum(value.as.list.items[i],&(*out)[i])) { free(*out); *out=NULL; return 0; } return 1; }
static OValue ostd_signal_conv(OValue left, OValue right, int correlate) { double *a,*b; size_t an,bn; if(!ostd_signal_data(left,&a,&an)||!ostd_signal_data(right,&b,&bn)) { free(a); free(b); return ofail("signal expects rank-one numeric data"); } size_t count=an&&bn?an+bn-1:0; double *out=count?(double *)calloc(count,sizeof(double)):NULL; if(count&&!out) { free(a); free(b); return ofail("out of memory"); } for(size_t i=0;i<count;i++) for(size_t j=0;j<an;j++) { size_t k=i>=j?i-j:SIZE_MAX; if(k<bn) out[i]+=a[j]*(correlate?b[bn-1-k]:b[k]); } free(a); free(b); size_t shape[1]={count}; return otensordata(out,count,shape,1); }
static OValue ostd_signal_stats(OValue value, int mode) { double *data; size_t n; if(!ostd_signal_data(value,&data,&n)||n==0) { free(data); return ofail("signal statistics expects non-empty numeric data"); } double sum=0,energy=0; for(size_t i=0;i<n;i++) { sum+=data[i]; energy+=data[i]*data[i]; } double mean=sum/n,result=mode==0?mean:mode==1?energy:mode==2?energy/n:sqrt(energy/n); if(mode==5) { double peak=0; for(size_t i=0;i<n;i++) if(fabs(data[i])>peak) peak=fabs(data[i]); if(peak==0) { free(data); return ofail("normalize peak is undefined for zero signal"); } for(size_t i=0;i<n;i++) data[i]/=peak; } else if(mode==4) { double lo=data[0],hi=data[0]; for(size_t i=1;i<n;i++) { if(data[i]<lo)lo=data[i]; if(data[i]>hi)hi=data[i]; } if(hi==lo) for(size_t i=0;i<n;i++) data[i]=0; else for(size_t i=0;i<n;i++) data[i]=(data[i]-lo)/(hi-lo); } free(data); if(mode>=4) return onull(); return ofloat(result); }
static OValue ostd_signal_fft(OValue value, int inverse) { double *data; size_t n; if(!ostd_signal_data(value,&data,&n)||n>4096) { free(data); return ofail("fft input exceeds the 4096-sample limit or is not numeric"); } OValue *items=n?(OValue *)malloc(n*sizeof(OValue)):NULL; if(n&&!items) { free(data); return ofail("out of memory"); } for(size_t k=0;k<n;k++) { double re=0,im=0; for(size_t t=0;t<n;t++) { double angle=(inverse?1:-1)*2*3.14159265358979323846*(double)k*(double)t/(double)(n?n:1); re+=data[t]*ogocos(angle); im+=data[t]*ogosin(angle); } if(inverse) { re/=n?n:1; im/=n?n:1; } items[k]=ocomplex(re,im); } free(data); OValue out=olist(items,n); free(items); return out; }
static OValue ostd_crypto_digest(OValue dataValue, OValue algorithm, int raw) { const char *name; size_t nl; const unsigned char *data; size_t len; unsigned char *owned=NULL; if(!ostd_string(algorithm,&name,&nl)||!ostd_bytes(dataValue,&data,&len,&owned)) { free(owned); return ofail("digest expects byte-compatible data and algorithm"); } int sha512=!strncmp(name,"sha512",nl); if(!sha512&&strncmp(name,"sha256",nl)) { free(owned); return ofail("unsupported crypto algorithm"); } unsigned char digest[64]; if(sha512) { free(owned); return ofail("sha512 native digest is not enabled in this portable build"); } OSHA256 state; oshainit(&state); oshawrite(&state,data,len); oshafinal(&state,digest); free(owned); if(raw) return ostd_bytes_list(digest,32); char hex[65]; static const char d[]="0123456789abcdef"; for(size_t i=0;i<32;i++){hex[2*i]=d[digest[i]>>4];hex[2*i+1]=d[digest[i]&15];} return oownedstring(hex,64); }
static OValue ostd_crypto_hex(OValue value, int decode) { const char *s; size_t n; const unsigned char *data; unsigned char *owned=NULL; if(decode) { if(!ostd_string(value,&s,&n)||n%2) return ofail("hex_decode expects an even-length string"); unsigned char *out=n?(unsigned char *)malloc(n/2):NULL; if(n&&!out) return ofail("out of memory"); for(size_t i=0;i<n;i+=2) { int a=(s[i]>='0'&&s[i]<='9')?s[i]-'0':(s[i]>='a'&&s[i]<='f')?s[i]-'a'+10:(s[i]>='A'&&s[i]<='F')?s[i]-'A'+10:-1; int b=(s[i+1]>='0'&&s[i+1]<='9')?s[i+1]-'0':(s[i+1]>='a'&&s[i+1]<='f')?s[i+1]-'a'+10:(s[i+1]>='A'&&s[i+1]<='F')?s[i+1]-'A'+10:-1; if(a<0||b<0) { free(out); return ofail("invalid hexadecimal encoding"); } out[i/2]=(unsigned char)((a<<4)|b); } OValue result=ostd_bytes_list(out,n/2); free(out); return result; } if(!ostd_bytes(value,&data,&n,&owned)) return ofail("hex_encode expects byte-compatible data"); char *out=n?(char *)malloc(n*2+1):NULL; if(n&&!out) { free(owned); return ofail("out of memory"); } static const char d[]="0123456789abcdef"; for(size_t i=0;i<n;i++){out[2*i]=d[data[i]>>4];out[2*i+1]=d[data[i]&15];} OValue result=oownedstring(out,n*2); free(out); free(owned); return result; }
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
static OValue ostd_dispatch(const char *module, const char *member, const OValue *args, size_t argc) {
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
    if(!strcmp(member,"chunks")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)||n<=0) return ofail("chunks size must be positive"); return ostd_text_chunks(args[0],n); }
    if(!strcmp(member,"compare")||!strcmp(member,"contains")||!strcmp(member,"has_prefix")||!strcmp(member,"has_suffix")||!strcmp(member,"equal_fold")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("text comparison expects two strings"); int cmp=0; if(!strcmp(member,"compare")) cmp=(args[0].as.s.len==args[1].as.s.len)?memcmp(args[0].as.s.data,args[1].as.s.data,args[0].as.s.len):args[0].as.s.len<args[1].as.s.len?-1:1; if(!strcmp(member,"contains")) return obool(args[1].as.s.len==0||strstr(args[0].as.s.data,args[1].as.s.data)!=NULL); if(!strcmp(member,"has_prefix")) return obool(args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data,args[1].as.s.data,args[1].as.s.len)); if(!strcmp(member,"has_suffix")) return obool(args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data+args[0].as.s.len-args[1].as.s.len,args[1].as.s.data,args[1].as.s.len)); if(!strcmp(member,"equal_fold")) { OValue a=ostd_text_case(args[0],0),b=ostd_text_case(args[1],0); if(a.kind==O_ERROR||b.kind==O_ERROR) return ofail("equal_fold expects valid UTF-8"); return obool(a.as.s.len==b.as.s.len&&!memcmp(a.as.s.data,b.as.s.data,a.as.s.len)); } return oint(cmp); }
    if(!strcmp(member,"count")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("count expects strings"); if(args[1].as.s.len==0) return ouint(ostd_runes(args[0].as.s.data,args[0].as.s.len)+1); size_t n=0,p=0; while((p=(size_t)(strstr(args[0].as.s.data+p,args[1].as.s.data)-args[0].as.s.data))<args[0].as.s.len) { n++; p+=args[1].as.s.len; } return ouint(n); }
    if(!strcmp(member,"index")||!strcmp(member,"last_index")||!strcmp(member,"index_any")||!strcmp(member,"last_index_any")) { if(argc!=2) return ofail("index expects 2 arguments"); return ostd_text_index(args[0],args[1],strstr(member,"last")!=NULL,strstr(member,"any")!=NULL); }
    if(!strcmp(member,"cut")||!strcmp(member,"cut_prefix")||!strcmp(member,"cut_suffix")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("cut expects strings"); const char *p=NULL; if(!strcmp(member,"cut_suffix")) { if(args[1].as.s.len<=args[0].as.s.len) { const char *q=args[0].as.s.data+args[0].as.s.len-args[1].as.s.len; if(!memcmp(q,args[1].as.s.data,args[1].as.s.len)) p=q; } if(!p) return ooptionNone(); return ooptionSome(ostringview(args[0].as.s.data,p-args[0].as.s.data)); } p=strstr(args[0].as.s.data,args[1].as.s.data); if(!p) return ooptionNone(); if(!strcmp(member,"cut_prefix")&&p!=args[0].as.s.data) return ooptionNone(); if(!strcmp(member,"cut_prefix")) return ooptionSome(ostringview(p+args[1].as.s.len,args[0].as.s.len-args[1].as.s.len)); OValue pair[2]={ostringview(args[0].as.s.data,p-args[0].as.s.data),ostringview(p+args[1].as.s.len,args[0].as.s.len-(size_t)(p-args[0].as.s.data)-args[1].as.s.len)}; return ooptionSome(olist(pair,2)); }
    if(!strcmp(member,"trim_left")||!strcmp(member,"trim_right")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("trim character set expects strings"); const char *s=args[0].as.s.data,*set=args[1].as.s.data; size_t start=0,end=args[0].as.s.len; while(start<end&&strchr(set,(unsigned char)s[start])) start++; while(end>start&&strchr(set,(unsigned char)s[end-1])) end--; return oownedstring(s+start,end-start); }
    if(!strcmp(member,"trim_prefix")||!strcmp(member,"trim_suffix")) { if(argc!=2||args[0].kind!=O_STRING||args[1].kind!=O_STRING) return ofail("trim prefix expects strings"); if(!strcmp(member,"trim_prefix")&&args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data,args[1].as.s.data,args[1].as.s.len)) return oownedstring(args[0].as.s.data+args[1].as.s.len,args[0].as.s.len-args[1].as.s.len); if(!strcmp(member,"trim_suffix")&&args[1].as.s.len<=args[0].as.s.len&&!memcmp(args[0].as.s.data+args[0].as.s.len-args[1].as.s.len,args[1].as.s.data,args[1].as.s.len)) return oownedstring(args[0].as.s.data,args[0].as.s.len-args[1].as.s.len); return oownedstring(args[0].as.s.data,args[0].as.s.len); }
  }
  if(!strcmp(module,"std/list")) { if(!strcmp(member,"range")) { if(argc!=1) return ofail("range expects 1 argument"); return ostd_list_range(args[0]); } if(!strcmp(member,"len")) { if(argc!=1) return ofail("len expects 1 argument"); if(args[0].kind==O_STRING) { size_t n=ostd_runes(args[0].as.s.data,args[0].as.s.len); return n==SIZE_MAX?ofail("len expects valid UTF-8"):ofloat((double)n); } if(args[0].kind==O_LIST) return ofloat((double)args[0].as.list.len); if(args[0].kind==O_TENSOR) return ofloat((double)args[0].as.tensor.len); return ofail("len expects a string, list, or tensor"); } if(!strcmp(member,"sum")||!strcmp(member,"product")) return argc==1?(!strcmp(member,"product")?ostd_list_product(args[0]):ostd_list_sum(args[0])):ofail("list reduction expects 1 argument"); if(!strcmp(member,"take")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)) return ofail("take expects a value and integer count"); return ostd_list_take(args[0],n); } if(!strcmp(member,"reverse")) return argc==1?ostd_list_reverse(args[0]):ofail("reverse expects 1 argument"); if(!strcmp(member,"flatten")) return argc==2?ostd_list_flatten(args[0],args[1]):ofail("flatten expects a list and depth"); if(!strcmp(member,"at")) return argc==2?ostd_list_at(args[0],args[1]):ofail("at expects 2 arguments"); if(!strcmp(member,"index_of")||!strcmp(member,"last_index_of")||!strcmp(member,"contains")||!strcmp(member,"count")) { if(argc!=2) return ofail("list search expects 2 arguments"); OValue result=ostd_list_index(args[0],args[1],!strcmp(member,"last_index_of"),!strcmp(member,"count")); if(!strcmp(member,"contains")) return obool(result.kind==O_INT&&result.as.i>=0); return result; } if(!strcmp(member,"sort")) return argc==1?ostd_list_sort(args[0]):ofail("sort expects 1 argument"); if(!strcmp(member,"unique")) return argc==1?ostd_list_unique(args[0]):ofail("unique expects 1 argument"); if(!strcmp(member,"slice")) { int64_t a,b; if(argc!=3||!ostd_i64(args[1],&a)||!ostd_i64(args[2],&b)) return ofail("slice expects list and integer bounds"); return ostd_list_shape(args[0],0,a,b); } if(!strcmp(member,"chunks")||!strcmp(member,"windows")) { int64_t a,b=0; if((!strcmp(member,"chunks")&&argc!=2)||(!strcmp(member,"windows")&&argc!=3)||!ostd_i64(args[1],&a)||(argc==3&&!ostd_i64(args[2],&b))) return ofail("list shape operation expects integer bounds"); return ostd_list_shape(args[0],!strcmp(member,"chunks")?1:2,a,b); } if(!strcmp(member,"zip")) return argc==2?ostd_list_zip(args[0],args[1]):ofail("zip expects 2 arguments"); if(!strcmp(member,"concat")) { size_t total=0; for(size_t i=0;i<argc;i++) { if(args[i].kind!=O_LIST||args[i].as.list.len>SIZE_MAX-total) return ofail("concat expects bounded lists"); total+=args[i].as.list.len; } OValue *items=total?(OValue *)malloc(total*sizeof(OValue)):NULL; if(total&&!items) return ofail("out of memory"); size_t at=0; for(size_t i=0;i<argc;i++){memcpy(items+at,args[i].as.list.items,args[i].as.list.len*sizeof(OValue));at+=args[i].as.list.len;} OValue out=olist(items,total);free(items);return out; } if(!strcmp(member,"repeat")) { int64_t n; if(argc!=2||!ostd_i64(args[1],&n)||n<0||n>100000) return ofail("repeat expects bounded item count"); OValue *items=n?(OValue *)malloc((size_t)n*sizeof(OValue)):NULL;if(n&&!items)return ofail("out of memory");for(int64_t i=0;i<n;i++)items[i]=args[0];OValue out=olist(items,(size_t)n);free(items);return out; } if(!strcmp(member,"empty")) return argc==0?olist(NULL,0):ofail("empty expects no arguments"); if(!strcmp(member,"all")||!strcmp(member,"any")||!strcmp(member,"none")) { if(argc!=1||args[0].kind!=O_LIST) return ofail("list predicate expects a list"); int wantAny=!strcmp(member,"any"),wantNone=!strcmp(member,"none"); for(size_t i=0;i<args[0].as.list.len;i++) { int truth=otruthy(args[0].as.list.items[i]); if((wantAny&&truth)||(!wantAny&&!wantNone&&!truth)||(wantNone&&truth)) return obool(wantAny?1:0); } return obool(!wantAny); } if(!strcmp(member,"enumerate")) { if(argc!=1||args[0].kind!=O_LIST) return ofail("enumerate expects 1 argument"); OValue *items=args[0].as.list.len?(OValue *)malloc(args[0].as.list.len*sizeof(OValue)):NULL;if(args[0].as.list.len&&!items)return ofail("out of memory");for(size_t i=0;i<args[0].as.list.len;i++){OValue p[2]={ouint(i),args[0].as.list.items[i]};items[i]=olist(p,2);}OValue out=olist(items,args[0].as.list.len);free(items);return out; } }
  if(!strcmp(module,"std/signal")) { if(!strcmp(member,"rectangular")||!strcmp(member,"hann")||!strcmp(member,"hamming")||!strcmp(member,"blackman")||!strcmp(member,"bartlett")) { if(argc!=1) return ofail("signal window expects 1 argument"); return ostd_signal_window(args[0],!strcmp(member,"rectangular")?0:!strcmp(member,"hann")?1:!strcmp(member,"hamming")?2:!strcmp(member,"blackman")?3:4); } if(!strcmp(member,"convolve")||!strcmp(member,"correlate")||!strcmp(member,"filter_fir")) { if(argc!=2) return ofail("signal operation expects 2 arguments"); return ostd_signal_conv(args[0],args[1],!strcmp(member,"correlate")); } if(!strcmp(member,"mean")||!strcmp(member,"energy")||!strcmp(member,"variance")||!strcmp(member,"rms")) { if(argc!=1) return ofail("signal statistic expects 1 argument"); return ostd_signal_stats(args[0],!strcmp(member,"mean")?0:!strcmp(member,"energy")?1:!strcmp(member,"variance")?2:3); } if(!strcmp(member,"fft")||!strcmp(member,"ifft")) return argc==1?ostd_signal_fft(args[0],!strcmp(member,"ifft")):ofail("fft expects 1 argument"); if(!strcmp(member,"normalize")) { if(argc!=2||args[1].kind!=O_STRING) return ofail("normalize expects signal and mode"); int mode=!strcmp(args[1].as.s.data,"minmax")?4:!strcmp(args[1].as.s.data,"peak")?5:-1; if(mode<0) return ofail("unknown normalization mode"); OValue result=ostd_signal_stats(args[0],mode); if(ohaderror) return result; double *data;size_t n;if(!ostd_signal_data(args[0],&data,&n))return ofail("normalize expects numeric signal"); if(mode==5){double peak=0;for(size_t i=0;i<n;i++)if(fabs(data[i])>peak)peak=fabs(data[i]);for(size_t i=0;i<n;i++)data[i]/=peak;}else{double lo=data[0],hi=data[0];for(size_t i=1;i<n;i++){if(data[i]<lo)lo=data[i];if(data[i]>hi)hi=data[i];}for(size_t i=0;i<n;i++)data[i]=hi==lo?0:(data[i]-lo)/(hi-lo);}size_t shape[1]={n};return otensordata(data,n,shape,1); } }
  if(!strcmp(module,"std/crypto")) { if(!strcmp(member,"hash")) { OValue rendered=ocat(args,argc); if(rendered.kind==O_ERROR)return rendered; return ostd_crypto_digest(rendered,ostring("sha256"),0); } if(!strcmp(member,"digest")||!strcmp(member,"digest_bytes")) return argc==2?ostd_crypto_digest(args[0],args[1],!strcmp(member,"digest_bytes")):ofail("digest expects data and algorithm"); if(!strcmp(member,"hex_encode")||!strcmp(member,"hex_decode")) return argc==1?ostd_crypto_hex(args[0],!strcmp(member,"hex_decode")):ofail("hex codec expects 1 argument"); if(!strcmp(member,"compare_digest")) { const unsigned char *a,*b;size_t an,bn;unsigned char *oa=NULL,*ob=NULL;if(argc!=2||!ostd_bytes(args[0],&a,&an,&oa)||!ostd_bytes(args[1],&b,&bn,&ob)){free(oa);free(ob);return ofail("compare_digest expects byte-compatible values");}unsigned char diff=(unsigned char)(an^bn);size_t n=an<bn?an:bn;for(size_t i=0;i<n;i++)diff|=a[i]^b[i];free(oa);free(ob);return obool(diff==0); } if(!strcmp(member,"algorithm_supported")) { if(argc!=1||args[0].kind!=O_STRING)return ofail("algorithm_supported expects a string");return obool(!strcmp(args[0].as.s.data,"sha256")||!strcmp(args[0].as.s.data,"sha512")||!strcmp(args[0].as.s.data,"sha512_256")); } }
  if(!strcmp(module,"std/time")) { if(!strcmp(member,"parse_duration")) return argc==1?ostd_time_parse_duration(args[0]):ofail("parse_duration expects 1 argument"); if(!strcmp(member,"duration_string")) return argc==1?ostd_time_duration_string(args[0]):ofail("duration_string expects 1 argument"); if(!strcmp(member,"add")) return argc==2?ostd_time_add(args[0],args[1]):ofail("add expects 2 arguments"); if(!strcmp(member,"sub")) return argc==2?ostd_time_sub(args[0],args[1]):ofail("sub expects 2 arguments"); if(!strcmp(member,"format")) return argc==2?ostd_time_format(args[0],args[1]):ofail("format expects 2 arguments"); if(!strcmp(member,"date")) return argc==8?ostd_time_date(args):ofail("date expects 8 arguments in native C"); if(!strcmp(member,"add_date")) return argc==4?ostd_time_add_date(args[0],args[1],args[2],args[3]):ofail("add_date expects 4 arguments"); }
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
