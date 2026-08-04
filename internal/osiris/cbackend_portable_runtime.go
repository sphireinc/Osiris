package osiris

// cRuntimePortableComponent contains portable tagged-ABI helpers shared by
// native programs that use standard-library, time, codec, and task values. It
// deliberately uses only the C11 library and cRuntimePrelude.
const cRuntimePortableComponent = `
static OValue oownedstring(const char *data, size_t len) {
  char *copy=(char *)malloc(len+1);
  if(!copy) return ofail("out of memory");
  memcpy(copy,data,len); copy[len]=0;
  if(!otrack(copy)) { free(copy); return ofail("out of memory"); }
  return (OValue){O_STRING,0,{.s={copy,len}}};
}
static OValue obytes(OValue value) {
  if(value.kind==O_LIST) return olist(value.as.list.items,value.as.list.len);
  if(value.kind!=O_STRING) return ofail("bytes expects a string or byte list");
  OValue *items=value.as.s.len?(OValue *)malloc(value.as.s.len*sizeof(OValue)):NULL;
  if(value.as.s.len&&!items) return ofail("out of memory");
  for(size_t i=0;i<value.as.s.len;i++) items[i]=ouintbits((unsigned char)value.as.s.data[i],8);
  OValue result=olist(items,value.as.s.len); free(items); return result;
}
static double ogotrig(double x, int cosine) {
  static const double sp[6]={1.58962301576546568060e-10,-2.50507477628578072866e-8,2.75573136213857245213e-6,-1.98412698295895385996e-4,8.33333333332211858878e-3,-1.66666666666666307295e-1};
  static const double cp[6]={-1.13585365213876817300e-11,2.08757008419747316778e-9,-2.75573141792967388112e-7,2.48015872888517045348e-5,-1.38888888888730564116e-3,4.16666666666665929218e-2};
  const double pi4a=7.85398125648498535156e-1,pi4b=3.77489470793079817668e-8,pi4c=2.69515142907905952645e-15;
  if(x==0&&!cosine) return x; if(isnan(x)||isinf(x)) return NAN;
  int sign=0; if(x<0) { x=-x; if(!cosine) sign=1; }
  uint64_t j=(uint64_t)(x*(4/3.14159265358979323846264338327950288)); double y=(double)j;
  if(j&1) { j++; y++; } j&=7;
  double z=((x-y*pi4a)-y*pi4b)-y*pi4c;
  if(cosine) { if(j>3){j-=4;sign=!sign;} if(j>1)sign=!sign; }
  else if(j>3){sign=!sign;j-=4;}
  double zz=z*z;
  if((cosine&& (j==1||j==2))||(!cosine&&(j!=1&&j!=2))) y=z+z*zz*(((((sp[0]*zz+sp[1])*zz+sp[2])*zz+sp[3])*zz+sp[4])*zz+sp[5]);
  else y=1.0-0.5*zz+zz*zz*(((((cp[0]*zz+cp[1])*zz+cp[2])*zz+cp[3])*zz+cp[4])*zz+cp[5]);
  return sign?-y:y;
}
static double ogosin(double value) { return ogotrig(value,0); }
static double ogocos(double value) { return ogotrig(value,1); }

static int onumericsequence(OValue value, size_t *count) {
  if(value.kind==O_TENSOR) { *count=value.as.tensor.len; return 1; }
  if(value.kind==O_LIST) { *count=value.as.list.len; return 1; }
  return 0;
}
static int onumericitem(OValue value, size_t index, double *out) {
  if(value.kind==O_TENSOR) { *out=value.as.tensor.data[index]; return 1; }
  return value.kind==O_LIST&&onum(value.as.list.items[index],out);
}

static OValue omean(OValue value) {
  size_t count; double item;
  if(!onumericsequence(value,&count)||count==0) return ofail("mean expects non-empty numeric data");
  double total=0; for(size_t i=0;i<count;i++) { if(!onumericitem(value,i,&item)) return ofail("mean expects numeric data"); total+=item; }
  return ofloat(total/(double)count);
}
static OValue ostd(OValue value) {
  size_t count; double item;
  if(!onumericsequence(value,&count)||count==0) return ofail("std expects non-empty numeric data");
  double total=0; for(size_t i=0;i<count;i++) { if(!onumericitem(value,i,&item)) return ofail("std expects numeric data"); total+=item; }
  double mean=total/(double)count,sum=0;
  for(size_t i=0;i<count;i++) { onumericitem(value,i,&item); double d=item-mean; sum+=d*d; }
  return ofloat(sqrt(sum/(double)count));
}
static int odoublecmp(const void *left, const void *right);
static OValue oquantile(OValue value, OValue qvalue);
static OValue ovar(OValue value) {
  size_t count; double item; if(!onumericsequence(value,&count)||count==0) return ofail("var expects non-empty numeric data");
  double total=0; for(size_t i=0;i<count;i++) { if(!onumericitem(value,i,&item)) return ofail("var expects numeric data"); total+=item; }
  double mean=total/(double)count,sum=0; for(size_t i=0;i<count;i++) { onumericitem(value,i,&item); double delta=item-mean; sum+=delta*delta; }
  return ofloat(sum/(double)count);
}
static OValue omedian(OValue value) {
  size_t count; if(!onumericsequence(value,&count)||count==0) return ofail("median expects non-empty numeric data");
  double *copy=(double *)malloc(count*sizeof(double)); if(!copy) return ofail("out of memory"); for(size_t i=0;i<count;i++) if(!onumericitem(value,i,&copy[i])) { free(copy); return ofail("median expects numeric data"); } qsort(copy,count,sizeof(double),odoublecmp);
  double result=count%2?copy[count/2]:(copy[count/2-1]+copy[count/2])/2; free(copy); return ofloat(result);
}
static OValue opercentile(OValue value, OValue p) { double percent; if(!onum(p,&percent)) return ofail("percentile expects data and p"); return oquantile(value,ofloat(percent/100)); }
static OValue oargextreme(OValue value, int want_max) {
  size_t count; double best,next; if(!onumericsequence(value,&count)||count==0||!onumericitem(value,0,&best)) return ofail(want_max?"argmax expects non-empty numeric data":"argmin expects non-empty numeric data"); size_t index=0;
  for(size_t i=1;i<count;i++) { if(!onumericitem(value,i,&next)) return ofail("argmin/argmax expects numeric data"); if((want_max&&next>best)||(!want_max&&next<best)) { best=next; index=i; } }
  return ouint(index);
}
static OValue oargmin(OValue value) { return oargextreme(value,0); }
static OValue oargmax(OValue value) { return oargextreme(value,1); }
static OValue oany(OValue value) { size_t count; if(!onumericsequence(value,&count)) return ofail("any expects data"); for(size_t i=0;i<count;i++) { double item; if(!onumericitem(value,i,&item)) return ofail("any expects numeric data"); if(item!=0) return obool(1); } return obool(0); }
static OValue oall(OValue value) { size_t count; if(!onumericsequence(value,&count)) return ofail("all expects data"); for(size_t i=0;i<count;i++) { double item; if(!onumericitem(value,i,&item)) return ofail("all expects numeric data"); if(item==0) return obool(0); } return obool(1); }
static OValue ocumulative(OValue value, int product) {
  size_t count; if(!onumericsequence(value,&count)) return ofail(product?"cumprod expects data":"cumsum expects data");
  double *data=count?(double *)malloc(count*sizeof(double)):NULL; if(count&&!data) return ofail("out of memory"); double total=product?1:0;
  for(size_t i=0;i<count;i++) { double item; if(!onumericitem(value,i,&item)) { free(data); return ofail("cumulative operation expects numeric data"); } total=product?total*item:total+item; data[i]=total; }
  if(value.kind==O_TENSOR) return otensordata(data,count,value.as.tensor.shape,value.as.tensor.rank);
  OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) { free(data); return ofail("out of memory"); } for(size_t i=0;i<count;i++) items[i]=ofloat(data[i]); free(data); OValue out=olist(items,count); free(items); return out;
}
static OValue ocumsum(OValue value) { return ocumulative(value,0); }
static OValue ocumprod(OValue value) { return ocumulative(value,1); }
static int odoublecmp(const void *left, const void *right) {
  double a=*(const double *)left,b=*(const double *)right;
  return a<b?-1:a>b?1:0;
}
static OValue oquantile(OValue value, OValue qvalue) {
  size_t count; double q;
  if(!onumericsequence(value,&count)||count==0||!onum(qvalue,&q)||q<0||q>1) return ofail("quantile expects non-empty numeric data and q in [0,1]");
  double *copy=(double *)malloc(count*sizeof(double)); if(!copy) return ofail("out of memory");
  for(size_t i=0;i<count;i++) if(!onumericitem(value,i,&copy[i])) { free(copy); return ofail("quantile expects numeric data"); } qsort(copy,count,sizeof(double),odoublecmp);
  double pos=q*(double)(count-1); size_t lo=(size_t)floor(pos),hi=(size_t)ceil(pos);
  double fraction=pos-(double)lo;
  double out=lo==hi?copy[lo]:fma(copy[hi],fraction,copy[lo]*(1-fraction));
  free(copy); return ofloat(out);
}
static OValue ophase(OValue value) {
  OComplex z; if(!ocomplexnum(value,&z)) return ofail("phase expects a complex-compatible value");
  return ofloatbits(atan2(z.imag,z.real),value.kind==O_COMPLEX?value.fbits:0);
}

static OValue ostd_list_range(OValue stopValue) {
  double stopNumber; if(!onum(stopValue,&stopNumber)||stopNumber<0||trunc(stopNumber)!=stopNumber||stopNumber>10000000) return ofail("range expects a non-negative bounded integer");
  size_t count=(size_t)stopNumber; OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL;
  if(count&&!items) return ofail("out of memory");
  for(size_t i=0;i<count;i++) items[i]=ofloat((double)i);
  OValue out=olist(items,count); free(items); return out;
}

static OValue ostd_signal_hann(OValue lengthValue) {
  double lengthNumber; if(!onum(lengthValue,&lengthNumber)||lengthNumber<0||trunc(lengthNumber)!=lengthNumber) return ofail("hann length must be a non-negative integer");
  size_t count=(size_t)lengthNumber; double *data=count?(double *)malloc(count*sizeof(double)):NULL;
  if(count&&!data) return ofail("out of memory");
  if(count==1) data[0]=1;
  else for(size_t i=0;i<count;i++) data[i]=0.5-0.5*ogocos(2*3.14159265358979323846264338327950288*(double)i/(double)(count-1));
  size_t shape[1]={count}; return otensordata(data,count,shape,1);
}
static int ovectorparts(OValue value, const double **data, size_t *count, double **owned) {
  *owned=NULL;
  if(value.kind==O_TENSOR&&value.as.tensor.rank==1) { *data=value.as.tensor.data; *count=value.as.tensor.len; return 1; }
  if(value.kind!=O_LIST) return 0;
  *count=value.as.list.len; *owned=*count?(double *)malloc(*count*sizeof(double)):NULL;
  if(*count&&!*owned) return 0;
  for(size_t i=0;i<*count;i++) if(!onum(value.as.list.items[i],&(*owned)[i])) { free(*owned); *owned=NULL; return 0; }
  *data=*owned; return 1;
}
static OValue ostd_signal_convolve(OValue left, OValue right) {
  const double *a,*b; double *oa=NULL,*ob=NULL; size_t an,bn;
  if(!ovectorparts(left,&a,&an,&oa)||!ovectorparts(right,&b,&bn,&ob)) { free(oa); free(ob); return ofail("convolve expects rank-one signal and kernel"); }
  size_t count=an&&bn?an+bn-1:0; double *data=count?(double *)calloc(count,sizeof(double)):NULL;
  if(count&&!data) { free(oa); free(ob); return ofail("out of memory"); }
  for(size_t i=0;i<count;i++) for(size_t j=0;j<an;j++) if(i>=j&&i-j<bn) data[i]+=a[j]*b[i-j];
  free(oa); free(ob); size_t shape[1]={count}; return otensordata(data,count,shape,1);
}

static size_t ocatpart(char *out, size_t cap, OValue value);
static int outf8decode(const char *data, size_t len, size_t *at, uint32_t *rune) {
  if(*at>=len) return 0; unsigned char first=(unsigned char)data[(*at)++];
  if(first<0x80) { *rune=first; return 1; }
  unsigned need; uint32_t value;
  if((first&0xe0)==0xc0) { need=1; value=first&0x1f; if(value<2) return 0; }
  else if((first&0xf0)==0xe0) { need=2; value=first&0x0f; }
  else if((first&0xf8)==0xf0) { need=3; value=first&0x07; if(value>4) return 0; }
  else return 0;
  if(*at+need>len) return 0;
  for(unsigned i=0;i<need;i++) { unsigned char next=(unsigned char)data[(*at)++]; if((next&0xc0)!=0x80) return 0; value=(value<<6)|(next&0x3f); }
  if((need==2&&value<0x800)||(need==3&&value<0x10000)||value>0x10ffff||(value>=0xd800&&value<=0xdfff)) return 0;
  *rune=value; return 1;
}
static size_t outf8encode(char *out, uint32_t rune) {
  if(rune<0x80) { out[0]=(char)rune; return 1; }
  if(rune<0x800) { out[0]=(char)(0xc0|(rune>>6)); out[1]=(char)(0x80|(rune&0x3f)); return 2; }
  if(rune<0x10000) { out[0]=(char)(0xe0|(rune>>12)); out[1]=(char)(0x80|((rune>>6)&0x3f)); out[2]=(char)(0x80|(rune&0x3f)); return 3; }
  out[0]=(char)(0xf0|(rune>>18)); out[1]=(char)(0x80|((rune>>12)&0x3f)); out[2]=(char)(0x80|((rune>>6)&0x3f)); out[3]=(char)(0x80|(rune&0x3f)); return 4;
}
static int ounicodespace(uint32_t r) {
  return (r>=0x09&&r<=0x0d)||r==0x20||r==0x85||r==0xa0||r==0x1680||(r>=0x2000&&r<=0x200a)||r==0x2028||r==0x2029||r==0x202f||r==0x205f||r==0x3000;
}
static int ounicodeletterdigit(uint32_t r) {
  return (r>='0'&&r<='9')||(r>='A'&&r<='Z')||(r>='a'&&r<='z')||(r>=0xc0&&r<=0x2af)||(r>=0x370&&r<=0x52f);
}
static uint32_t ounicodelower(uint32_t r) {
  if(r>='A'&&r<='Z') return r+32;
  if((r>=0xc0&&r<=0xd6)||(r>=0xd8&&r<=0xde)) return r+32;
  if(r>=0x391&&r<=0x3ab&&r!=0x3a2) return r+32;
  return r;
}
static uint32_t ounicodeupper(uint32_t r) {
  if(r>='a'&&r<='z') return r-32;
  if((r>=0xe0&&r<=0xf6)||(r>=0xf8&&r<=0xfe)) return r-32;
  if(r>=0x3b1&&r<=0x3cb&&r!=0x3c2) return r-32;
  if(r==0x3c2) return 0x3a3;
  return r;
}
static OValue ostd_text_slug(OValue value) {
  if(value.kind!=O_STRING) return ofail("slug expects a string");
  char *out=(char *)malloc(value.as.s.len*2+1); if(!out) return ofail("out of memory");
  size_t at=0,n=0; int dash=0; uint32_t rune;
  while(at<value.as.s.len) {
    if(!outf8decode(value.as.s.data,value.as.s.len,&at,&rune)) { free(out); return ofail("slug expects valid UTF-8"); }
    rune=ounicodelower(rune);
    if(ounicodeletterdigit(rune)) { n+=outf8encode(out+n,rune); dash=0; }
    else if(!dash&&n) { out[n++]='-'; dash=1; }
  }
  if(n&&out[n-1]=='-') n--; OValue result=oownedstring(out,n); free(out); return result;
}
static OValue ostd_text_fields(OValue value) {
  if(value.kind!=O_STRING) return ofail("fields expects a string");
  size_t count=0,i=0,start=0; uint32_t rune; int infield=0;
  while(i<value.as.s.len) { if(!outf8decode(value.as.s.data,value.as.s.len,&i,&rune)) return ofail("fields expects valid UTF-8"); if(ounicodespace(rune)) infield=0; else if(!infield) { count++; infield=1; } }
  OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) return ofail("out of memory");
  i=0; size_t item=0; infield=0;
  while(i<value.as.s.len) { size_t before=i; outf8decode(value.as.s.data,value.as.s.len,&i,&rune); if(ounicodespace(rune)) { if(infield) { items[item++]=ostringview(value.as.s.data+start,before-start); infield=0; } } else if(!infield) { start=before; infield=1; } }
  if(infield) items[item++]=ostringview(value.as.s.data+start,value.as.s.len-start);
  OValue result=olist(items,count); free(items); return result;
}
static OValue ostd_text_upper(OValue value) {
  if(value.kind!=O_STRING) return ofail("upper expects a string");
  char *out=(char *)malloc(value.as.s.len*2+1); if(!out) return ofail("out of memory");
  size_t at=0,n=0; uint32_t rune;
  while(at<value.as.s.len) { if(!outf8decode(value.as.s.data,value.as.s.len,&at,&rune)) { free(out); return ofail("upper expects valid UTF-8"); } if(rune==0xdf) { out[n++]='S'; out[n++]='S'; } else n+=outf8encode(out+n,ounicodeupper(rune)); }
  OValue result=oownedstring(out,n); free(out); return result;
}
static OValue ostd_text_join(OValue separator, OValue values) {
  if(separator.kind!=O_STRING||values.kind!=O_LIST) return ofail("join expects a string separator and list");
  size_t len=0; for(size_t i=0;i<values.as.list.len;i++) len+=ocatpart(NULL,0,values.as.list.items[i])+(i?separator.as.s.len:0);
  char *out=(char *)malloc(len+1); if(!out) return ofail("out of memory"); size_t at=0;
  for(size_t i=0;i<values.as.list.len;i++) { if(i) { memcpy(out+at,separator.as.s.data,separator.as.s.len); at+=separator.as.s.len; } at+=ocatpart(out+at,len-at,values.as.list.items[i]); }
  OValue result=oownedstring(out,len); free(out); return result;
}

static uint64_t orol64(uint64_t value, unsigned shift) { return (value<<shift)|(value>>(64-shift)); }

static int64_t odays_from_civil(int year, unsigned month, unsigned day) {
  year-=month<=2; int era=(year>=0?year:year-399)/400; unsigned yoe=(unsigned)(year-era*400);
  unsigned doy=(153*(month+(month>2?(unsigned)-3:9))+2)/5+day-1; unsigned doe=yoe*365+yoe/4-yoe/100+doy;
  return (int64_t)era*146097+(int64_t)doe-719468;
}
static void ocivil_from_days(int64_t z, int *year, unsigned *month, unsigned *day) {
  z+=719468; int64_t era=(z>=0?z:z-146096)/146097; unsigned doe=(unsigned)(z-era*146097);
  unsigned yoe=(doe-doe/1460+doe/36524-doe/146096)/365; int y=(int)yoe+(int)era*400;
  unsigned doy=doe-(365*yoe+yoe/4-yoe/100),mp=(5*doy+2)/153;
  *day=doy-(153*mp+2)/5+1; *month=mp+(mp<10?3:(unsigned)-9); *year=y+(*month<=2);
}
static int ointarg(OValue value, int *out) { double n; if(!onum(value,&n)||trunc(n)!=n||n<-2147483648.0||n>2147483647.0) return 0; *out=(int)n; return 1; }
static int oleapyear(int year) { return year%4==0&&(year%100!=0||year%400==0); }
static int odaysinmonth(int year, int month) {
  static const int lengths[12]={31,28,31,30,31,30,31,31,30,31,30,31};
  return month==2&&oleapyear(year)?29:lengths[month-1];
}
static OValue ostd_time_date(const OValue *args) {
  int y,m,d,h,min,s,ns; if(!ointarg(args[0],&y)||!ointarg(args[1],&m)||!ointarg(args[2],&d)||!ointarg(args[3],&h)||!ointarg(args[4],&min)||!ointarg(args[5],&s)||!ointarg(args[6],&ns)) return ofail("date expects integer civil fields");
  if(y<1||y>9999||m<1||m>12||d<1||d>odaysinmonth(y,m)||h<0||h>23||min<0||min>59||s<0||s>59||ns<0||ns>999999999) return ofail("invalid civil datetime");
  int64_t seconds=odays_from_civil(y,(unsigned)m,(unsigned)d)*86400+(int64_t)h*3600+(int64_t)min*60+s;
  return (OValue){O_DATETIME,0,{.i=seconds*1000000000+ns}};
}
static OValue ostd_time_add_date(OValue date, OValue yearsValue, OValue monthsValue, OValue daysValue) {
  int years,months,days; if(date.kind!=O_DATETIME||!ointarg(yearsValue,&years)||!ointarg(monthsValue,&months)||!ointarg(daysValue,&days)) return ofail("add_date expects datetime and integer offsets");
  int64_t seconds=date.as.i/1000000000,dayNumber=seconds/86400; int y; unsigned m,d; ocivil_from_days(dayNumber,&y,&m,&d);
  int total=y*12+(int)m-1+years*12+months; int ny=total>=0?total/12:(total-11)/12,nm=total-ny*12+1;
  int64_t tod=seconds-dayNumber*86400; return (OValue){O_DATETIME,0,{.i=(odays_from_civil(ny,(unsigned)nm,d)+days)*86400000000000+tod*1000000000+date.as.i%1000000000}};
}
static OValue ostd_time_parse_duration(OValue value) {
  if(value.kind!=O_STRING) return ofail("parse_duration expects a string"); int64_t total=0,current=0; size_t i=0;
  if(value.as.s.len==0) return ofail("invalid duration");
  while(i<value.as.s.len) { if(value.as.s.data[i]<'0'||value.as.s.data[i]>'9') return ofail("invalid duration"); current=0; while(i<value.as.s.len&&value.as.s.data[i]>='0'&&value.as.s.data[i]<='9') { int digit=value.as.s.data[i++]-'0'; if(current>(INT64_MAX-digit)/10) return ofail("duration overflow"); current=current*10+digit; } if(i>=value.as.s.len) return ofail("invalid duration"); char unit=value.as.s.data[i++]; int64_t scale=unit=='h'?3600000000000LL:unit=='m'?60000000000LL:unit=='s'?1000000000LL:0,part; if(!scale||__builtin_mul_overflow(current,scale,&part)||__builtin_add_overflow(total,part,&total)) return ofail(scale?"duration overflow":"invalid duration"); }
  return (OValue){O_DURATION,0,{.i=total}};
}
static OValue ostd_time_add(OValue date, OValue duration) { int64_t result; return date.kind==O_DATETIME&&duration.kind==O_DURATION&&!__builtin_add_overflow(date.as.i,duration.as.i,&result)?(OValue){O_DATETIME,0,{.i=result}}:ofail("add expects datetime and duration without overflow"); }
static OValue ostd_time_sub(OValue left, OValue right) { int64_t result; return left.kind==O_DATETIME&&right.kind==O_DATETIME&&!__builtin_sub_overflow(left.as.i,right.as.i,&result)?(OValue){O_DURATION,0,{.i=result}}:ofail("sub expects datetimes without overflow"); }
static OValue ostd_time_format(OValue date, OValue layout) {
  if(date.kind!=O_DATETIME||layout.kind!=O_STRING) return ofail("format expects datetime and layout strings"); int64_t seconds=date.as.i/1000000000,nanos=date.as.i%1000000000; if(nanos<0) { seconds--; nanos+=1000000000; } int64_t days=seconds/86400,tod=seconds-days*86400; if(tod<0) { days--; tod+=86400; } int y; unsigned m,d; ocivil_from_days(days,&y,&m,&d);
  char text[40]; int n=snprintf(text,sizeof(text),"%04d-%02u-%02uT%02lld:%02lld:%02lld",y,m,d,(long long)(tod/3600),(long long)(tod/60%60),(long long)(tod%60));
  if(nanos&&layout.as.s.len==0) { char fraction[10]; snprintf(fraction,sizeof(fraction),"%09lld",(long long)nanos); size_t used=9; while(used&&fraction[used-1]=='0') used--; text[n++]='.'; memcpy(text+n,fraction,used); n+=(int)used; }
  text[n++]='Z'; text[n]=0; return oownedstring(text,(size_t)n);
}
static OValue ostd_time_duration_string(OValue duration) {
  if(duration.kind!=O_DURATION) return ofail("duration_string expects duration"); int64_t signedns=duration.as.i; char text[96]; size_t at=0;
  uint64_t ns=signedns<0?(uint64_t)(-(signedns+1))+1:(uint64_t)signedns; if(signedns<0) text[at++]='-';
  uint64_t hours=ns/3600000000000ULL; ns%=3600000000000ULL;
  uint64_t minutes=ns/60000000000ULL; ns%=60000000000ULL;
  uint64_t seconds=ns/1000000000ULL, fraction=ns%1000000000ULL;
  if(hours) at+=(size_t)snprintf(text+at,sizeof(text)-at,"%lluh",(unsigned long long)hours);
  if(hours||minutes) at+=(size_t)snprintf(text+at,sizeof(text)-at,"%llum",(unsigned long long)minutes);
  if(hours||minutes||seconds||fraction==0) {
    at+=(size_t)snprintf(text+at,sizeof(text)-at,"%llu",(unsigned long long)seconds);
    if(fraction) { char digits[10]; snprintf(digits,sizeof(digits),"%09llu",(unsigned long long)fraction); size_t used=9; while(used&&digits[used-1]=='0') used--; text[at++]='.'; memcpy(text+at,digits,used); at+=used; }
    text[at++]='s';
  } else if(ns>=1000000ULL&&ns%1000000ULL==0) at+=(size_t)snprintf(text+at,sizeof(text)-at,"%llums",(unsigned long long)(ns/1000000ULL));
  else if(ns>=1000ULL&&ns%1000ULL==0) at+=(size_t)snprintf(text+at,sizeof(text)-at,"%lluus",(unsigned long long)(ns/1000ULL));
  else at+=(size_t)snprintf(text+at,sizeof(text)-at,"%lluns",(unsigned long long)ns);
  int n=(int)at;
  return oownedstring(text,(size_t)n);
}
static void oprintdatetime(OValue date) {
  OValue text=ostd_time_format(date,ostring(""));
  printf("datetime(\"%.*s\")",(int)text.as.s.len,text.as.s.data);
}

static size_t ofloattext(char *text, size_t cap, double value);
static size_t ocatpart(char *out, size_t cap, OValue value) {
  char text[128]; const char *data=text; int n=0;
  switch(value.kind) {
  case O_STRING: data=value.as.s.data; n=(int)value.as.s.len; break;
  case O_ERROR: data=value.as.error.message.data; n=(int)value.as.error.message.len; break;
  case O_UINT: n=snprintf(text,sizeof(text),"%lluu%u",(unsigned long long)value.as.u,(unsigned)(value.fbits?value.fbits:64)); break;
  case O_INT: n=snprintf(text,sizeof(text),"%lldi%u",(long long)value.as.i,(unsigned)(value.fbits?value.fbits:64)); break;
  case O_FLOAT: n=(int)ofloattext(text,sizeof(text),value.as.f); if(value.fbits) n+=snprintf(text+n,sizeof(text)-(size_t)n,"f%u",(unsigned)value.fbits); break;
  case O_COMPLEX:
    if(value.as.cx.real==0) { n=(int)ofloattext(text,sizeof(text),value.as.cx.imag); text[n++]='i'; }
    else { n=(int)ofloattext(text,sizeof(text),value.as.cx.real); if(signbit(value.as.cx.imag)) { text[n++]='-'; n+=(int)ofloattext(text+n,sizeof(text)-(size_t)n,-value.as.cx.imag); } else { text[n++]='+'; n+=(int)ofloattext(text+n,sizeof(text)-(size_t)n,value.as.cx.imag); } text[n++]='i'; }
    break;
  default: n=snprintf(text,sizeof(text),"<value>"); break;
  }
  if(out&&cap>=(size_t)n) memcpy(out,data,(size_t)n); return (size_t)n;
}
static OValue ocat(const OValue *values, size_t count) {
  size_t len=0; for(size_t i=0;i<count;i++) len+=ocatpart(NULL,0,values[i]);
  char *out=(char *)malloc(len+1); if(!out) return ofail("out of memory"); size_t at=0;
  for(size_t i=0;i<count;i++) at+=ocatpart(out+at,len-at,values[i]);
  OValue result=oownedstring(out,len); free(out); return result;
}

typedef struct { unsigned char *data; size_t len,cap; } OBuffer;
static int obufferadd(OBuffer *buffer, const void *data, size_t len) {
  if(len>SIZE_MAX-buffer->len) return 0;
  size_t need=buffer->len+len;
  if(need>buffer->cap) { size_t cap=buffer->cap?buffer->cap:128; while(cap<need) { if(cap>SIZE_MAX/2) { cap=need; break; } cap*=2; } unsigned char *grown=(unsigned char *)realloc(buffer->data,cap); if(!grown) return 0; buffer->data=grown; buffer->cap=cap; }
  memcpy(buffer->data+buffer->len,data,len); buffer->len=need; return 1;
}
static int obuffertext(OBuffer *buffer, const char *text) { return obufferadd(buffer,text,strlen(text)); }

#define OCODEC_LIMIT (64u*1024u*1024u)
#define OCODEC_DEPTH 256u
typedef struct { const unsigned char *data; size_t len,off; } OCursor;
static int ocodecbyte(OBuffer *buffer, unsigned char value) { return obufferadd(buffer,&value,1); }
static int ocodecuvar(OBuffer *buffer, uint64_t value) {
  unsigned char raw[10]; size_t count=0;
  while(value>=0x80) { raw[count++]=(unsigned char)(value|0x80); value>>=7; }
  raw[count++]=(unsigned char)value; return obufferadd(buffer,raw,count);
}
static int ocodecvar(OBuffer *buffer, int64_t value) {
  uint64_t encoded=(uint64_t)value<<1; if(value<0) encoded=~encoded;
  return ocodecuvar(buffer,encoded);
}
static int ocodecu64be(OBuffer *buffer, uint64_t value) {
  unsigned char raw[8]; for(size_t i=0;i<8;i++) raw[i]=(unsigned char)(value>>(56-8*i));
  return obufferadd(buffer,raw,8);
}
static int ocodecencodeinto(OBuffer *buffer, OValue value, unsigned depth) {
  if(depth>OCODEC_DEPTH||buffer->len>OCODEC_LIMIT) return 0;
  switch(value.kind) {
  case O_NULL: return ocodecbyte(buffer,0x00);
  case O_BOOL: return ocodecbyte(buffer,value.as.b?0x02:0x01);
  case O_UINT:
    return ocodecbyte(buffer,0x04)&&ocodecbyte(buffer,(unsigned char)(value.fbits?value.fbits:64))&&ocodecu64be(buffer,value.as.u);
  case O_INT: {
    char text[32]; size_t len=(size_t)snprintf(text,sizeof(text),"%lld",(long long)value.as.i);
    return ocodecbyte(buffer,0x0e)&&ocodecbyte(buffer,(unsigned char)(value.fbits?value.fbits:64))&&ocodecuvar(buffer,len)&&obufferadd(buffer,text,len);
  }
  case O_FLOAT: {
    uint64_t bits; memcpy(&bits,&value.as.f,sizeof(bits));
    if(value.fbits) { if(!ocodecbyte(buffer,0x10)||!ocodecbyte(buffer,(unsigned char)value.fbits)) return 0; }
    else if(!ocodecbyte(buffer,0x03)) return 0;
    return ocodecu64be(buffer,bits);
  }
  case O_COMPLEX: {
    uint64_t realbits,imagbits; memcpy(&realbits,&value.as.cx.real,sizeof(realbits)); memcpy(&imagbits,&value.as.cx.imag,sizeof(imagbits));
    if(value.fbits) { if(!ocodecbyte(buffer,0x18)||!ocodecbyte(buffer,(unsigned char)value.fbits)) return 0; }
    else if(!ocodecbyte(buffer,0x08)) return 0;
    return ocodecu64be(buffer,realbits)&&ocodecu64be(buffer,imagbits);
  }
  case O_DUAL: {
    uint64_t primalbits,tangentbits; memcpy(&primalbits,&value.as.dual.primal,sizeof(primalbits)); memcpy(&tangentbits,&value.as.dual.tangent,sizeof(tangentbits));
    return ocodecbyte(buffer,0x19)&&ocodecu64be(buffer,primalbits)&&ocodecu64be(buffer,tangentbits);
  }
  case O_DECIMAL:
  case O_BIGINT:
    return ocodecbyte(buffer,value.kind==O_DECIMAL?0x07:0x0b)&&ocodecuvar(buffer,value.as.s.len)&&obufferadd(buffer,value.as.s.data,value.as.s.len);
  case O_STRING:
    return ocodecbyte(buffer,0x05)&&ocodecuvar(buffer,value.as.s.len)&&obufferadd(buffer,value.as.s.data,value.as.s.len);
  case O_LIST:
    if(!ocodecbyte(buffer,0x06)||!ocodecuvar(buffer,value.as.list.len)) return 0;
    for(size_t i=0;i<value.as.list.len;i++) if(!ocodecencodeinto(buffer,value.as.list.items[i],depth+1)) return 0;
    return 1;
  case O_TENSOR: {
    if(!ocodecbyte(buffer,0x09)||!ocodecuvar(buffer,value.as.tensor.rank)) return 0;
    for(size_t i=0;i<value.as.tensor.rank;i++) if(!ocodecuvar(buffer,value.as.tensor.shape[i])) return 0;
    if(!ocodecuvar(buffer,value.as.tensor.len)) return 0;
    for(size_t i=0;i<value.as.tensor.len;i++) { OValue item=value.as.tensor.element_kind==1?ouintbits((uint64_t)value.as.tensor.data[i],value.fbits?value.fbits:64):value.as.tensor.element_kind==2?ointbits((int64_t)value.as.tensor.data[i],value.fbits?value.fbits:64):ofloatbits(value.as.tensor.data[i],value.fbits); if(!ocodecencodeinto(buffer,item,depth+1)) return 0; }
    return 1;
  }
  case O_COMPLEX_TENSOR: {
    if(!ocodecbyte(buffer,0x09)||!ocodecuvar(buffer,value.as.ctensor.rank)) return 0;
    for(size_t i=0;i<value.as.ctensor.rank;i++) if(!ocodecuvar(buffer,value.as.ctensor.shape[i])) return 0;
    if(!ocodecuvar(buffer,value.as.ctensor.len)) return 0;
    for(size_t i=0;i<value.as.ctensor.len;i++) { OValue item=ocomplexbits(value.as.ctensor.data[i].real,value.as.ctensor.data[i].imag,value.fbits); if(!ocodecencodeinto(buffer,item,depth+1)) return 0; }
    return 1;
  }
  case O_QUANTITY: {
    if(value.as.quantity.unit.len==0) return 0;
    char magnitude[64]; int length=snprintf(magnitude,sizeof(magnitude),"%.17g",value.as.quantity.magnitude); if(length<0||(size_t)length>=sizeof(magnitude)) return 0;
    return ocodecbyte(buffer,0x0d)&&ocodecuvar(buffer,(size_t)length)&&obufferadd(buffer,magnitude,(size_t)length)&&ocodecuvar(buffer,1)&&ocodecuvar(buffer,value.as.quantity.unit.len)&&obufferadd(buffer,value.as.quantity.unit.data,value.as.quantity.unit.len)&&ocodecvar(buffer,1);
  }
  case O_DATETIME: {
    int64_t seconds=value.as.i/1000000000,nanos=value.as.i%1000000000;
    if(nanos<0) { seconds--; nanos+=1000000000; }
    return ocodecbyte(buffer,0x1c)&&ocodecvar(buffer,seconds)&&ocodecuvar(buffer,(uint64_t)nanos)&&ocodecbyte(buffer,0)&&ocodecuvar(buffer,3)&&obuffertext(buffer,"UTC");
  }
  case O_DURATION:
    return ocodecbyte(buffer,0x1a)&&ocodecvar(buffer,value.as.i);
  default:
    return 0;
  }
}
static OValue oencode(OValue value) {
  OBuffer buffer={0};
  if(!obuffertext(&buffer,"OSR1")||!ocodecencodeinto(&buffer,value,0)||buffer.len>OCODEC_LIMIT) { free(buffer.data); return ofail("cannot encode native value"); }
  OValue result=oownedstring((const char *)buffer.data,buffer.len); free(buffer.data); return result;
}
static int ocursortake(OCursor *cursor, size_t count, const unsigned char **out) {
  if(count>cursor->len-cursor->off) return 0; *out=cursor->data+cursor->off; cursor->off+=count; return 1;
}
static int ocursorbyte(OCursor *cursor, unsigned char *out) {
  const unsigned char *raw; if(!ocursortake(cursor,1,&raw)) return 0; *out=*raw; return 1;
}
static int ocursoruvar(OCursor *cursor, uint64_t *out) {
  size_t start=cursor->off; uint64_t value=0; unsigned shift=0; unsigned char byte=0;
  for(size_t i=0;i<10;i++) { if(!ocursorbyte(cursor,&byte)||(i==9&&byte>1)) return 0; value|=(uint64_t)(byte&0x7f)<<shift; if(!(byte&0x80)) { size_t used=cursor->off-start,minimal=1; for(uint64_t n=value;n>=0x80;n>>=7) minimal++; if(used!=minimal) return 0; *out=value; return 1; } shift+=7; }
  return 0;
}
static int ocursorvar(OCursor *cursor, int64_t *out) {
  uint64_t raw; if(!ocursoruvar(cursor,&raw)) return 0;
  uint64_t magnitude=raw>>1; *out=(raw&1)?-(int64_t)magnitude-1:(int64_t)magnitude; return 1;
}
static int ocursoru64be(OCursor *cursor, uint64_t *out) {
  const unsigned char *raw; if(!ocursortake(cursor,8,&raw)) return 0; uint64_t value=0;
  for(size_t i=0;i<8;i++) value=(value<<8)|raw[i]; *out=value; return 1;
}
static int ocodecdecodefrom(OCursor *cursor, OValue *out, unsigned depth) {
  if(depth>OCODEC_DEPTH) return 0; unsigned char tag;
  if(!ocursorbyte(cursor,&tag)) return 0;
  switch(tag) {
  case 0x00: *out=onull(); return 1;
  case 0x01: *out=obool(0); return 1;
  case 0x02: *out=obool(1); return 1;
  case 0x03:
  case 0x10: {
    unsigned char width=0; uint64_t bits; if(tag==0x10&&(!ocursorbyte(cursor,&width)||(width!=32&&width!=64))) return 0;
    if(!ocursoru64be(cursor,&bits)) return 0; double value; memcpy(&value,&bits,sizeof(value)); *out=ofloatbits(value,width); return 1;
  }
  case 0x04: {
    unsigned char width; uint64_t value; if(!ocursorbyte(cursor,&width)||width==0||width>64||!ocursoru64be(cursor,&value)) return 0;
    if(width<64&&value>omask(width)) return 0; *out=ouintbits(value,width); return 1;
  }
  case 0x0e: {
    unsigned char width; uint64_t len; const unsigned char *raw;
    if(!ocursorbyte(cursor,&width)||width==0||width>64||!ocursoruvar(cursor,&len)||len==0||len>32||!ocursortake(cursor,(size_t)len,&raw)) return 0;
    char text[33]; memcpy(text,raw,(size_t)len); text[len]=0; char *end=NULL; long long value=strtoll(text,&end,10);
    if(!end||*end||((value==LLONG_MIN||value==LLONG_MAX)&&strcmp(text,"-9223372036854775808")&&strcmp(text,"9223372036854775807"))) return 0;
    *out=ointbits((int64_t)value,width); return 1;
  }
  case 0x08:
  case 0x18: {
    unsigned char width=0; uint64_t realbits,imagbits; if(tag==0x18&&(!ocursorbyte(cursor,&width)||(width!=32&&width!=64))) return 0;
    if(!ocursoru64be(cursor,&realbits)||!ocursoru64be(cursor,&imagbits)) return 0;
    double real,imag; memcpy(&real,&realbits,sizeof(real)); memcpy(&imag,&imagbits,sizeof(imag)); *out=ocomplexbits(real,imag,width); return 1;
  }
  case 0x19: {
    uint64_t primalbits,tangentbits; if(!ocursoru64be(cursor,&primalbits)||!ocursoru64be(cursor,&tangentbits)) return 0;
    double primal,tangent; memcpy(&primal,&primalbits,sizeof(primal)); memcpy(&tangent,&tangentbits,sizeof(tangent)); *out=odual(primal,tangent); return 1;
  }
  case 0x07:
  case 0x0b: {
    uint64_t len; const unsigned char *raw; if(!ocursoruvar(cursor,&len)||len==0||len>OCODEC_LIMIT||!ocursortake(cursor,(size_t)len,&raw)) return 0;
    OValue text=ostringview((const char *)raw,(size_t)len); *out=oexacttext(tag==0x07?O_DECIMAL:O_BIGINT,text); return !ohaderror;
  }
  case 0x05: {
    uint64_t len; const unsigned char *raw; if(!ocursoruvar(cursor,&len)||len>OCODEC_LIMIT||!ocursortake(cursor,(size_t)len,&raw)) return 0;
    *out=oownedstring((const char *)raw,(size_t)len); return !ohaderror;
  }
  case 0x06: {
    uint64_t count; if(!ocursoruvar(cursor,&count)||count>OCODEC_LIMIT/sizeof(OValue)) return 0;
    OValue *items=count?(OValue *)malloc((size_t)count*sizeof(OValue)):NULL; if(count&&!items) return 0;
    for(uint64_t i=0;i<count;i++) if(!ocodecdecodefrom(cursor,&items[i],depth+1)) { free(items); return 0; }
    *out=olist(items,(size_t)count); free(items); return !ohaderror;
  }
  case 0x09: {
    uint64_t rank,count; if(!ocursoruvar(cursor,&rank)||rank>OCODEC_DEPTH) return 0;
    size_t *shape=rank?(size_t *)malloc((size_t)rank*sizeof(size_t)):NULL; if(rank&&!shape) return 0;
    size_t expected=1; for(size_t i=0;i<(size_t)rank;i++) { uint64_t dim; if(!ocursoruvar(cursor,&dim)||dim>SIZE_MAX||(dim&&expected>SIZE_MAX/(size_t)dim)) { free(shape); return 0; } shape[i]=(size_t)dim; expected*=shape[i]; }
    if(!ocursoruvar(cursor,&count)||count!=expected||count>OCODEC_LIMIT/sizeof(OValue)) { free(shape); return 0; }
    OValue *items=count?(OValue *)malloc((size_t)count*sizeof(OValue)):NULL; if(count&&!items) { free(shape); return 0; }
    int complex=0; unsigned char element_kind=0; for(size_t i=0;i<(size_t)count;i++) { if(!ocodecdecodefrom(cursor,&items[i],depth+1)) { free(items); free(shape); return 0; } if(items[i].kind==O_COMPLEX) complex=1; else if(items[i].kind==O_UINT) { if(element_kind&&element_kind!=1) { free(items); free(shape); return 0; } element_kind=1; } else if(items[i].kind==O_INT) { if(element_kind&&element_kind!=2) { free(items); free(shape); return 0; } element_kind=2; } else if(items[i].kind!=O_FLOAT) { free(items); free(shape); return 0; } }
    if(complex) { OComplex *data=count?(OComplex *)malloc((size_t)count*sizeof(OComplex)):NULL; if(count&&!data) { free(items); free(shape); return 0; } for(size_t i=0;i<(size_t)count;i++) { if(!ocomplexnum(items[i],&data[i])) { free(data); free(items); free(shape); return 0; } } *out=ocomplextensordata(data,(size_t)count,shape,(size_t)rank); }
    else { double *data=count?(double *)malloc((size_t)count*sizeof(double)):NULL; if(count&&!data) { free(items); free(shape); return 0; } for(size_t i=0;i<(size_t)count;i++) data[i]=element_kind==1?(double)items[i].as.u:element_kind==2?(double)items[i].as.i:items[i].as.f; *out=otensordata_kind(data,(size_t)count,shape,(size_t)rank,count&&items[0].kind==O_FLOAT?items[0].fbits:0,element_kind); }
    free(items); free(shape); return !ohaderror;
  }
  case 0x0d: {
    uint64_t magLen,dimCount,unitLen; const unsigned char *mag,*unit; int64_t exponent;
    if(!ocursoruvar(cursor,&magLen)||magLen==0||magLen>128||!ocursortake(cursor,(size_t)magLen,&mag)||!ocursoruvar(cursor,&dimCount)||dimCount!=1||!ocursoruvar(cursor,&unitLen)||unitLen==0||unitLen>128||!ocursortake(cursor,(size_t)unitLen,&unit)||!ocursorvar(cursor,&exponent)||exponent!=1) return 0;
    char number[129]; memcpy(number,mag,(size_t)magLen); number[magLen]=0; char *end=NULL; double magnitude=strtod(number,&end); if(!end||*end) return 0;
    *out=(OValue){O_QUANTITY,0,{.quantity={magnitude,{(const char *)unit,(size_t)unitLen}}}}; return 1;
  }
  case 0x1a: {
    int64_t value; if(!ocursorvar(cursor,&value)) return 0; *out=(OValue){O_DURATION,0,{.i=value}}; return 1;
  }
  case 0x1c: {
    int64_t seconds; uint64_t nanos,nameLen; unsigned char classValue; const unsigned char *name;
    if(!ocursorvar(cursor,&seconds)||!ocursoruvar(cursor,&nanos)||nanos>999999999||!ocursorbyte(cursor,&classValue)||classValue!=0||!ocursoruvar(cursor,&nameLen)||nameLen!=3||!ocursortake(cursor,3,&name)||memcmp(name,"UTC",3)) return 0;
    if(seconds>INT64_MAX/1000000000||seconds<INT64_MIN/1000000000) return 0;
    *out=(OValue){O_DATETIME,0,{.i=seconds*1000000000+(int64_t)nanos}}; return 1;
  }
  default: return 0;
  }
}
static OValue odecode(OValue value) {
  const unsigned char *data=NULL; size_t len=0; unsigned char *owned=NULL;
  if(value.kind==O_STRING) { data=(const unsigned char *)value.as.s.data; len=value.as.s.len; }
  else if(value.kind==O_LIST) {
    len=value.as.list.len; owned=len?(unsigned char *)malloc(len):NULL; if(len&&!owned) return ofail("out of memory");
    for(size_t i=0;i<len;i++) { OValue item=value.as.list.items[i]; if(item.kind!=O_UINT||item.as.u>255) { free(owned); return ofail("decode expects encoded bytes"); } owned[i]=(unsigned char)item.as.u; }
    data=owned;
  } else return ofail("decode expects encoded bytes");
  if(len>OCODEC_LIMIT) { free(owned); return ofail("invalid encoded value"); }
  OCursor cursor={data,len,0}; const unsigned char *header; OValue result;
  int valid=ocursortake(&cursor,4,&header)&&!memcmp(header,"OSR1",4)&&ocodecdecodefrom(&cursor,&result,0)&&cursor.off==cursor.len;
  free(owned); return valid?result:ofail("invalid encoded value");
}

static size_t ofloattext(char *text, size_t cap, double value) {
  if(isnan(value)) return (size_t)snprintf(text,cap,"NaN");
  if(isinf(value)) return (size_t)snprintf(text,cap,signbit(value)?"-Inf":"+Inf");
  if(trunc(value)==value&&value>=-9223372036854775808.0&&value<=9223372036854775807.0) return (size_t)snprintf(text,cap,"%lld",(long long)value);
  char trial[64],*end=NULL;
  for(int precision=1;precision<=17;precision++) { snprintf(trial,sizeof(trial),"%.*g",precision,value); double roundtrip=strtod(trial,&end); if(end&&*end==0&&roundtrip==value&&!(value==0&&signbit(roundtrip)!=signbit(value))) return (size_t)snprintf(text,cap,"%s",trial); }
  return (size_t)snprintf(text,cap,"%.17g",value);
}
static int orender(OBuffer *buffer, OValue value) {
  char text[128]; size_t n;
  switch(value.kind) {
  case O_NULL: return obuffertext(buffer,"null");
  case O_BOOL: return obuffertext(buffer,value.as.b?"true":"false");
  case O_UINT: n=(size_t)snprintf(text,sizeof(text),"%lluu%u",(unsigned long long)value.as.u,(unsigned)(value.fbits?value.fbits:64)); return obufferadd(buffer,text,n);
  case O_INT: n=(size_t)snprintf(text,sizeof(text),"%lldi%u",(long long)value.as.i,(unsigned)(value.fbits?value.fbits:64)); return obufferadd(buffer,text,n);
  case O_FLOAT: n=ofloattext(text,sizeof(text),value.as.f); if(!obufferadd(buffer,text,n)) return 0; if(value.fbits) { n=(size_t)snprintf(text,sizeof(text),"f%u",(unsigned)value.fbits); return obufferadd(buffer,text,n); } return 1;
  case O_STRING:
    if(!obuffertext(buffer,"\"")) return 0;
    for(size_t i=0;i<value.as.s.len;i++) { unsigned char c=(unsigned char)value.as.s.data[i]; if(c=='\"'||c=='\\') { char escaped[2]={'\\',(char)c}; if(!obufferadd(buffer,escaped,2)) return 0; } else if(c=='\n') { if(!obuffertext(buffer,"\\n")) return 0; } else if(c=='\r') { if(!obuffertext(buffer,"\\r")) return 0; } else if(c=='\t') { if(!obuffertext(buffer,"\\t")) return 0; } else if(!obufferadd(buffer,&c,1)) return 0; }
    return obuffertext(buffer,"\"");
  case O_LIST:
    if(!obuffertext(buffer,"[")) return 0; for(size_t i=0;i<value.as.list.len;i++) { if(i&&!obuffertext(buffer,", ")) return 0; if(!orender(buffer,value.as.list.items[i])) return 0; } return obuffertext(buffer,"]");
  case O_ENUM:
    if(!obufferadd(buffer,value.as.en.type_name.data,value.as.en.type_name.len)||!obuffertext(buffer,".")||!obufferadd(buffer,value.as.en.case_name.data,value.as.en.case_name.len)) return 0;
    if(value.as.en.payload.len) { if(!obuffertext(buffer,"(")) return 0; for(size_t i=0;i<value.as.en.payload.len;i++) { if(i&&!obuffertext(buffer,", ")) return 0; if(!orender(buffer,value.as.en.payload.items[i])) return 0; } if(!obuffertext(buffer,")")) return 0; } return 1;
  case O_COMPLEX:
    n=ofloattext(text,sizeof(text),value.as.cx.real); if(!obufferadd(buffer,text,n)) return 0; if(value.as.cx.imag>=0&&!obuffertext(buffer,"+")) return 0; n=ofloattext(text,sizeof(text),value.as.cx.imag); return obufferadd(buffer,text,n)&&obuffertext(buffer,"i");
  case O_DATETIME:
    n=(size_t)snprintf(text,sizeof(text),"datetime(%lld)",(long long)value.as.i); return obufferadd(buffer,text,n);
  case O_DURATION:
    n=(size_t)snprintf(text,sizeof(text),"duration(%lld)",(long long)value.as.i); return obufferadd(buffer,text,n);
  case O_DECIMAL: return obufferadd(buffer,value.as.s.data,value.as.s.len)&&obuffertext(buffer,"d");
  case O_BIGINT: return obufferadd(buffer,value.as.s.data,value.as.s.len)&&obuffertext(buffer,"n");
  case O_QUANTITY: n=ofloattext(text,sizeof(text),value.as.quantity.magnitude); return obufferadd(buffer,text,n)&&obuffertext(buffer,"*")&&obufferadd(buffer,value.as.quantity.unit.data,value.as.quantity.unit.len);
  default: return 0;
  }
}
static OValue ocrypto_random(OValue seed, OValue counterValue) {
  OBuffer rendered={0}; if(!orender(&rendered,seed)) { free(rendered.data); return ofail("random cannot render seed"); }
  uint64_t state=UINT64_C(0xcbf29ce484222325);
  for(size_t i=0;i<rendered.len;i++) { state^=rendered.data[i]; state*=UINT64_C(0x100000001b3); state=orol64(state,11)^(state>>5); }
  free(rendered.data);
  double counter; if(!onum(counterValue,&counter)||counter<0||trunc(counter)!=counter) return ofail("random counter must be a non-negative integer");
  state^=(uint64_t)counter+UINT64_C(0x9e3779b97f4a7c15);
  state^=state>>30; state*=UINT64_C(0xbf58476d1ce4e5b9); state^=state>>27; state*=UINT64_C(0x94d049bb133111eb); state^=state>>31;
  return ofloat((double)(state>>11)*(1.0/9007199254740992.0));
}

typedef struct { uint32_t h[8]; uint64_t bits; unsigned char block[64]; size_t used; } OSHA256;
static uint32_t osharor(uint32_t x, unsigned n) { return (x>>n)|(x<<(32-n)); }
static void oshablock(OSHA256 *s, const unsigned char *p) {
  static const uint32_t k[64]={0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2};
  uint32_t w[64]; for(size_t i=0;i<16;i++) w[i]=(uint32_t)p[i*4]<<24|(uint32_t)p[i*4+1]<<16|(uint32_t)p[i*4+2]<<8|p[i*4+3];
  for(size_t i=16;i<64;i++) { uint32_t a=osharor(w[i-15],7)^osharor(w[i-15],18)^(w[i-15]>>3),b=osharor(w[i-2],17)^osharor(w[i-2],19)^(w[i-2]>>10); w[i]=w[i-16]+a+w[i-7]+b; }
  uint32_t a=s->h[0],b=s->h[1],c=s->h[2],d=s->h[3],e=s->h[4],f=s->h[5],g=s->h[6],h=s->h[7];
  for(size_t i=0;i<64;i++) { uint32_t s1=osharor(e,6)^osharor(e,11)^osharor(e,25),ch=(e&f)^((~e)&g),t1=h+s1+ch+k[i]+w[i],s0=osharor(a,2)^osharor(a,13)^osharor(a,22),maj=(a&b)^(a&c)^(b&c),t2=s0+maj; h=g;g=f;f=e;e=d+t1;d=c;c=b;b=a;a=t1+t2; }
  s->h[0]+=a;s->h[1]+=b;s->h[2]+=c;s->h[3]+=d;s->h[4]+=e;s->h[5]+=f;s->h[6]+=g;s->h[7]+=h;
}
static void oshainit(OSHA256 *s) { uint32_t initial[8]={0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19}; memcpy(s->h,initial,sizeof(initial)); s->bits=0;s->used=0; }
static void oshawrite(OSHA256 *s, const unsigned char *data, size_t len) { s->bits+=(uint64_t)len*8; while(len) { size_t take=64-s->used; if(take>len)take=len; memcpy(s->block+s->used,data,take);s->used+=take;data+=take;len-=take;if(s->used==64){oshablock(s,s->block);s->used=0;} } }
static void oshafinal(OSHA256 *s, unsigned char out[32]) { s->block[s->used++]=0x80; if(s->used>56){while(s->used<64)s->block[s->used++]=0;oshablock(s,s->block);s->used=0;} while(s->used<56)s->block[s->used++]=0; for(int i=7;i>=0;i--)s->block[s->used++]=(unsigned char)(s->bits>>(i*8)); oshablock(s,s->block); for(size_t i=0;i<8;i++){out[i*4]=(unsigned char)(s->h[i]>>24);out[i*4+1]=(unsigned char)(s->h[i]>>16);out[i*4+2]=(unsigned char)(s->h[i]>>8);out[i*4+3]=(unsigned char)s->h[i];} }
static OValue ocrypto_hash(const OValue *values, size_t count) {
  OSHA256 sha; oshainit(&sha);
  for(size_t i=0;i<count;i++) { OBuffer rendered={0}; if(!orender(&rendered,values[i])) { free(rendered.data); return ofail("hash cannot render native value"); } oshawrite(&sha,rendered.data,rendered.len); unsigned char zero=0; oshawrite(&sha,&zero,1); free(rendered.data); }
  unsigned char digest[32]; char hex[65]; static const char digits[]="0123456789abcdef"; oshafinal(&sha,digest); for(size_t i=0;i<32;i++){hex[i*2]=digits[digest[i]>>4];hex[i*2+1]=digits[digest[i]&15];} return oownedstring(hex,64);
}
`
