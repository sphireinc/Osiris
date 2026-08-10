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

/* Tensor shape/value operations use the same row-major, owned representation
   as the core tensor kernels.  Inputs may be a tensor, a rectangular numeric
   list, or a numeric scalar, matching inferTensor in the language runtime.
   These helpers intentionally cover real numeric values only; complex tensors
   continue to use the separate complex runtime contract. */
static int oreal_tensor_input(OValue value, OValue *normalized) {
  if(value.kind==O_TENSOR) { *normalized=value; return 1; }
  *normalized=otensor(value);
  return normalized->kind==O_TENSOR;
}
static int oaxisvalue(OValue value, int64_t *out) {
  double number;
  if(value.kind==O_INT) { *out=value.as.i; return 1; }
  if(value.kind==O_UINT) { if(value.as.u>INT64_MAX) return 0; *out=(int64_t)value.as.u; return 1; }
  if(value.kind!=O_FLOAT||!onum(value,&number)||!isfinite(number)||floor(number)!=number||number<(double)INT64_MIN||number>(double)INT64_MAX) return 0;
  *out=(int64_t)number;
  return 1;
}
static int otensor_product_checked(const size_t *shape, size_t rank, size_t *out) {
  size_t total=1;
  for(size_t i=0;i<rank;i++) {
    if(shape[i]&&total>SIZE_MAX/shape[i]) return 0;
    total*=shape[i];
  }
  *out=total;
  return 1;
}
static OValue otensor_copy_shape(OValue input, const size_t *shape, size_t rank) {
  size_t n=input.as.tensor.len;
  if(n>SIZE_MAX/sizeof(double)) return ofail("tensor is too large");
  double *data=n?(double *)malloc(n*sizeof(double)):NULL;
  if(n&&!data) return ofail("out of memory");
  if(n) memcpy(data,input.as.tensor.data,n*sizeof(double));
  unsigned char element_kind=input.as.tensor.element_kind;
  return otensordata_kind(data,n,shape,rank,input.fbits,element_kind);
}
static OValue otensor_squeeze(const OValue *args, size_t argc) {
  if(argc!=1) return ofail("squeeze expects 1 argument");
  OValue input;
  if(!oreal_tensor_input(args[0],&input)) return ofail("squeeze expects a real numeric tensor");
  size_t rank=0;
  for(size_t i=0;i<input.as.tensor.rank;i++) if(input.as.tensor.shape[i]!=1) rank++;
  size_t *shape=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL;
  if(rank&&!shape) return ofail("out of memory");
  size_t at=0;
  for(size_t i=0;i<input.as.tensor.rank;i++) if(input.as.tensor.shape[i]!=1) shape[at++]=input.as.tensor.shape[i];
  OValue out=otensor_copy_shape(input,shape,rank);
  free(shape);
  return out;
}
static OValue otensor_expand_dims(const OValue *args, size_t argc) {
  if(argc!=1&&argc!=2) return ofail("expand_dims expects 1 or 2 arguments");
  OValue input;
  if(!oreal_tensor_input(args[0],&input)) return ofail("expand_dims expects a real numeric tensor");
  int64_t axis=0;
  if(argc==2&&!oaxisvalue(args[1],&axis)) return ofail("expand_dims axis must be an integer");
  if(axis<0) {
    if(input.as.tensor.rank>INT64_MAX-1) return ofail("expand_dims axis out of range");
    axis+=(int64_t)input.as.tensor.rank+1;
  }
  if(axis<0||(uint64_t)axis>input.as.tensor.rank) return ofail("expand_dims axis out of range");
  size_t rank=input.as.tensor.rank+1;
  if(rank<input.as.tensor.rank) return ofail("tensor rank is too large");
  size_t *shape=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL;
  if(rank&&!shape) return ofail("out of memory");
  size_t insert=(size_t)axis;
  for(size_t i=0,j=0;i<rank;i++) shape[i]=i==insert?1:input.as.tensor.shape[j++];
  OValue out=otensor_copy_shape(input,shape,rank);
  free(shape);
  return out;
}
static int otensor_target_compatible(OTensor input, const size_t *target, size_t target_rank, size_t *target_len) {
  if(target_rank<input.rank) return 0;
  size_t total;
  if(!otensor_product_checked(target,target_rank,&total)) return 0;
  for(size_t axis=0;axis<target_rank;axis++) {
    size_t source_dim=1;
    if(axis+input.rank>=target_rank) source_dim=input.shape[axis-(target_rank-input.rank)];
    if(source_dim==0&&target[axis]!=0) return 0;
    if(source_dim!=target[axis]&&source_dim!=1&&target[axis]!=1) return 0;
    size_t result_dim=source_dim>target[axis]?source_dim:target[axis];
    if(result_dim!=target[axis]) return 0;
  }
  *target_len=total;
  return 1;
}
static size_t otensor_broadcast_index(OTensor input, const size_t *target, size_t target_rank, const size_t *strides, size_t output) {
  size_t rest=output,index=0;
  for(size_t axis=target_rank;axis>0;axis--) {
    size_t at=axis-1, coordinate=target[at]?rest%target[at]:0;
    if(target[at]) rest/=target[at];
    if(at+input.rank>=target_rank) {
      size_t source_axis=at-(target_rank-input.rank);
      if(input.shape[source_axis]!=1) index+=coordinate*strides[source_axis];
    }
  }
  return index;
}
static OValue otensor_broadcast_to(const OValue *args, size_t argc) {
  if(argc<2) return ofail("broadcast_to expects a value and target shape");
  OValue input;
  if(!oreal_tensor_input(args[0],&input)) return ofail("broadcast_to expects a real numeric tensor");
  size_t *target=NULL,target_rank=0,target_len=0;
  if(!oshapeargs(args+1,argc-1,&target,&target_rank,&target_len)) return ofail("shape dimensions must be non-negative integers");
  if(!otensor_target_compatible(input.as.tensor,target,target_rank,&target_len)) { free(target); return ofail("broadcast shape mismatch"); }
  size_t *strides=input.as.tensor.rank?(size_t *)malloc(input.as.tensor.rank*sizeof(size_t)):NULL;
  if(input.as.tensor.rank&&!strides) { free(target); return ofail("out of memory"); }
  size_t stride=1;
  for(size_t i=input.as.tensor.rank;i>0;i--) { strides[i-1]=stride; if(input.as.tensor.shape[i-1]&&stride>SIZE_MAX/input.as.tensor.shape[i-1]) { free(strides); free(target); return ofail("tensor is too large"); } stride*=input.as.tensor.shape[i-1]; }
  if(target_len>SIZE_MAX/sizeof(double)) { free(strides); free(target); return ofail("tensor is too large"); }
  double *data=target_len?(double *)malloc(target_len*sizeof(double)):NULL;
  if(target_len&&!data) { free(strides); free(target); return ofail("out of memory"); }
  for(size_t i=0;i<target_len;i++) data[i]=input.as.tensor.data[otensor_broadcast_index(input.as.tensor,target,target_rank,strides,i)];
  OValue out=otensordata_kind(data,target_len,target,target_rank,input.fbits,input.as.tensor.element_kind);
  free(strides); free(target);
  return out;
}
static OValue oclip_value(OValue value, double lo, double hi) {
  if(value.kind==O_LIST) {
    OValue *items=value.as.list.len?(OValue *)malloc(value.as.list.len*sizeof(OValue)):NULL;
    if(value.as.list.len&&!items) return ofail("out of memory");
    for(size_t i=0;i<value.as.list.len;i++) { items[i]=oclip_value(value.as.list.items[i],lo,hi); if(items[i].kind==O_ERROR) { free(items); return items[i]; } }
    OValue out=olist(items,value.as.list.len); free(items); return out;
  }
  if(value.kind==O_TENSOR) {
    size_t n=value.as.tensor.len;
    if(n>SIZE_MAX/sizeof(double)) return ofail("tensor is too large");
    double *data=n?(double *)malloc(n*sizeof(double)):NULL;
    if(n&&!data) return ofail("out of memory");
    for(size_t i=0;i<n;i++) { double v=value.as.tensor.data[i]; if(v<lo) v=lo; if(v>hi) v=hi; data[i]=v; }
    return otensordata_kind(data,n,value.as.tensor.shape,value.as.tensor.rank,value.fbits,value.as.tensor.element_kind);
  }
  double number;
  if(!onum(value,&number)) return ofail("clip expects numeric values");
  if(number<lo) number=lo;
  if(number>hi) number=hi;
  return value.kind==O_FLOAT?ofloatbits(number,value.fbits):ofloat(number);
}
static OValue otensor_clip(const OValue *args, size_t argc) {
  if(argc!=3) return ofail("clip expects 3 arguments");
  double lo,hi;
  if(!onum(args[1],&lo)||!onum(args[2],&hi)) return ofail("clip expects numeric bounds");
  return oclip_value(args[0],lo,hi);
}
static int otensor_common_broadcast_shape(OTensor left, OTensor right, size_t **shape, size_t *rank, size_t *len) {
  return otensorbroadcastshape(left,right,shape,rank,len);
}
static OValue otensor_where(const OValue *args, size_t argc) {
  if(argc!=3) return ofail("where expects 3 arguments");
  OValue condition,when_true,when_false;
  if(!oreal_tensor_input(args[0],&condition)||!oreal_tensor_input(args[1],&when_true)||!oreal_tensor_input(args[2],&when_false)) return ofail("where expects real numeric tensors");
  size_t *shape=NULL,*final_shape=NULL,rank=0,final_rank=0,n=0,final_n=0;
  if(!otensor_common_broadcast_shape(condition.as.tensor,when_true.as.tensor,&shape,&rank,&n)) return ofail("tensor shape mismatch");
  OTensor intermediate={NULL,n,shape,rank,0};
  if(!otensor_common_broadcast_shape(intermediate,when_false.as.tensor,&final_shape,&final_rank,&final_n)) { free(shape); return ofail("tensor shape mismatch"); }
  free(shape);
  size_t *condition_strides=condition.as.tensor.rank?(size_t *)malloc(condition.as.tensor.rank*sizeof(size_t)):NULL;
  size_t *true_strides=when_true.as.tensor.rank?(size_t *)malloc(when_true.as.tensor.rank*sizeof(size_t)):NULL;
  size_t *false_strides=when_false.as.tensor.rank?(size_t *)malloc(when_false.as.tensor.rank*sizeof(size_t)):NULL;
  if((condition.as.tensor.rank&&!condition_strides)||(when_true.as.tensor.rank&&!true_strides)||(when_false.as.tensor.rank&&!false_strides)) { free(condition_strides); free(true_strides); free(false_strides); free(final_shape); return ofail("out of memory"); }
  size_t stride=1;
  for(size_t i=condition.as.tensor.rank;i>0;i--) { condition_strides[i-1]=stride; stride*=condition.as.tensor.shape[i-1]; }
  stride=1;
  for(size_t i=when_true.as.tensor.rank;i>0;i--) { true_strides[i-1]=stride; stride*=when_true.as.tensor.shape[i-1]; }
  stride=1;
  for(size_t i=when_false.as.tensor.rank;i>0;i--) { false_strides[i-1]=stride; stride*=when_false.as.tensor.shape[i-1]; }
  if(final_n>SIZE_MAX/sizeof(double)) { free(condition_strides); free(true_strides); free(false_strides); free(final_shape); return ofail("tensor is too large"); }
  double *data=final_n?(double *)malloc(final_n*sizeof(double)):NULL;
  if(final_n&&!data) { free(condition_strides); free(true_strides); free(false_strides); free(final_shape); return ofail("out of memory"); }
  for(size_t i=0;i<final_n;i++) {
    size_t ci=otensor_broadcast_index(condition.as.tensor,final_shape,final_rank,condition_strides,i);
    size_t ti=otensor_broadcast_index(when_true.as.tensor,final_shape,final_rank,true_strides,i);
    size_t fi=otensor_broadcast_index(when_false.as.tensor,final_shape,final_rank,false_strides,i);
    data[i]=condition.as.tensor.data[ci]!=0?when_true.as.tensor.data[ti]:when_false.as.tensor.data[fi];
  }
  uint16_t fbits=when_true.fbits>when_false.fbits?when_true.fbits:when_false.fbits;
  unsigned char element_kind=when_true.as.tensor.element_kind==when_false.as.tensor.element_kind?when_true.as.tensor.element_kind:0;
  OValue out=otensordata_kind(data,final_n,final_shape,final_rank,fbits,element_kind);
  free(condition_strides); free(true_strides); free(false_strides); free(final_shape);
  return out;
}
static int otensor_normalized_shape(OValue value, size_t **shape, size_t *rank) {
  if(value.kind==O_TENSOR&&value.as.tensor.rank>0) { *rank=value.as.tensor.rank; *shape=(size_t *)malloc(value.as.tensor.rank*sizeof(size_t)); if(!*shape) return 0; memcpy(*shape,value.as.tensor.shape,*rank*sizeof(size_t)); return 1; }
  *shape=(size_t *)malloc(sizeof(size_t)); if(!*shape) return 0; (*shape)[0]=1; *rank=1; return 1;
}
static OValue otensor_concat(const OValue *args, size_t argc) {
  if(argc<2) return ofail("concat expects at least 2 arguments");
  OValue *inputs=(OValue *)malloc(argc*sizeof(OValue)); size_t **shapes=(size_t **)calloc(argc,sizeof(size_t *)),*ranks=(size_t *)calloc(argc,sizeof(size_t));
  if(!inputs||!shapes||!ranks) { free(inputs); free(shapes); free(ranks); return ofail("out of memory"); }
  for(size_t i=0;i<argc;i++) if(!oreal_tensor_input(args[i],&inputs[i])||!otensor_normalized_shape(inputs[i],&shapes[i],&ranks[i])) { for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("concat expects real numeric tensors"); }
  size_t tail_rank=ranks[0]-1,total=0; if(ranks[0]==0) tail_rank=0;
  for(size_t i=0;i<argc;i++) {
    if(ranks[i]==0) { for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("concat expects arrays, got scalar"); }
    if(ranks[i]-1!=tail_rank||(tail_rank&&memcmp(shapes[i]+1,shapes[0]+1,tail_rank*sizeof(size_t)))) { for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("concat shape mismatch"); }
    if(shapes[i][0]>SIZE_MAX-total) { for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("tensor is too large"); }
    total+=shapes[i][0];
  }
  size_t out_rank=ranks[0],*out_shape=out_rank?(size_t *)malloc(out_rank*sizeof(size_t)):NULL;
  if(out_rank&&!out_shape) { for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("out of memory"); }
  out_shape[0]=total; for(size_t i=1;i<out_rank;i++) out_shape[i]=shapes[0][i];
  size_t n; if(!otensor_product_checked(out_shape,out_rank,&n)||n>SIZE_MAX/sizeof(double)) { free(out_shape); for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("tensor is too large"); }
  double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) { free(out_shape); for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return ofail("out of memory"); }
  size_t at=0; uint16_t fbits=0; unsigned char element_kind=inputs[0].as.tensor.element_kind;
  for(size_t i=0;i<argc;i++) { if(inputs[i].fbits>fbits) fbits=inputs[i].fbits; if(inputs[i].as.tensor.element_kind!=element_kind) element_kind=0; if(inputs[i].as.tensor.len) { memcpy(data+at,inputs[i].as.tensor.data,inputs[i].as.tensor.len*sizeof(double)); at+=inputs[i].as.tensor.len; } }
  OValue out=otensordata_kind(data,n,out_shape,out_rank,fbits,element_kind); free(out_shape); for(size_t j=0;j<argc;j++) free(shapes[j]); free(inputs); free(shapes); free(ranks); return out;
}
static OValue otensor_stack(const OValue *args, size_t argc) {
  if(argc<2) return ofail("stack expects at least 2 arguments");
  OValue *inputs=(OValue *)malloc(argc*sizeof(OValue)); if(!inputs) return ofail("out of memory");
  for(size_t i=0;i<argc;i++) if(!oreal_tensor_input(args[i],&inputs[i])) { free(inputs); return ofail("stack expects real numeric tensors"); }
  OTensor first=inputs[0].as.tensor;
  for(size_t i=1;i<argc;i++) if(!otensorsameshape(first,inputs[i].as.tensor)) { free(inputs); return ofail("stack shape mismatch"); }
  size_t out_rank=first.rank+1,*out_shape=out_rank?(size_t *)malloc(out_rank*sizeof(size_t)):NULL;
  if(out_rank&&!out_shape) { free(inputs); return ofail("out of memory"); }
  out_shape[0]=argc; for(size_t i=0;i<first.rank;i++) out_shape[i+1]=first.shape[i];
  size_t n; if(!otensor_product_checked(out_shape,out_rank,&n)||n>SIZE_MAX/sizeof(double)) { free(out_shape); free(inputs); return ofail("tensor is too large"); }
  double *data=n?(double *)malloc(n*sizeof(double)):NULL; if(n&&!data) { free(out_shape); free(inputs); return ofail("out of memory"); }
  size_t at=0; uint16_t fbits=0; unsigned char element_kind=first.element_kind;
  for(size_t i=0;i<argc;i++) { if(inputs[i].fbits>fbits) fbits=inputs[i].fbits; if(inputs[i].as.tensor.element_kind!=element_kind) element_kind=0; if(inputs[i].as.tensor.len) { memcpy(data+at,inputs[i].as.tensor.data,inputs[i].as.tensor.len*sizeof(double)); at+=inputs[i].as.tensor.len; } }
  OValue out=otensordata_kind(data,n,out_shape,out_rank,fbits,element_kind); free(out_shape); free(inputs); return out;
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

static double onative_degrees(double value) { return value*(180.0/3.14159265358979323846264338327950288); }
static double onative_radians(double value) { return value*(3.14159265358979323846264338327950288/180.0); }
static double onative_copysign(double magnitude, double sign) { return copysign(magnitude,sign); }
static double onative_nextafter(double from, double to) { return nextafter(from,to); }
static float onative_nextafterf(float from, float to) { return nextafterf(from,to); }
static int onative_isfinite(double value) { return isfinite(value)!=0; }
static int onative_isinf(double value) { return isinf(value)!=0; }
static int onative_isnan(double value) { return isnan(value)!=0; }
static int onative_isnormal(double value) { return value!=0.0&&isfinite(value)&&fabs(value)>=DBL_MIN; }
static int onative_isnormalf(float value) { return value!=0.0f&&isfinite(value)&&fabsf(value)>=FLT_MIN; }
static int onative_signbit(double value) { return signbit(value)!=0; }

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
static int odecimal_integer(OString text, int64_t *out) {
  size_t at=0; int negative=0; uint64_t value=0;
  if(at<text.len&&(text.data[at]=='+'||text.data[at]=='-')) { negative=text.data[at]=='-'; at++; }
  size_t digits=0; int fractional=0;
  for(;at<text.len;at++) {
    char c=text.data[at];
    if(c=='d'||c=='n') { if(at+1!=text.len) return 0; break; }
    if(c=='.'&&!fractional) { fractional=1; continue; }
    if(c<'0'||c>'9') return 0;
    if(fractional&&c!='0') return 0;
    if(!fractional) {
      unsigned digit=(unsigned)(c-'0');
      if(value>(UINT64_MAX-digit)/10) return 0;
      value=value*10+digit; digits++;
    }
  }
  if(digits==0) return 0;
  if(negative) {
    if(value>UINT64_C(9223372036854775808)) return 0;
    *out=value==UINT64_C(9223372036854775808)?INT64_MIN:-(int64_t)value;
  } else {
    if(value>UINT64_C(9223372036854775807)) return 0;
    *out=(int64_t)value;
  }
  return 1;
}
static int ointarg(OValue value, int64_t *out) {
  if(value.kind==O_INT) { *out=value.as.i; return 1; }
  if(value.kind==O_UINT) { if(value.as.u>UINT64_C(9223372036854775807)) return 0; *out=(int64_t)value.as.u; return 1; }
  if(value.kind==O_DECIMAL||value.kind==O_BIGINT) return odecimal_integer(value.as.s,out);
  if(value.kind==O_FLOAT) {
    double n=value.as.f;
    if(!isfinite(n)||trunc(n)!=n||n<(-0x1p63)||n>=0x1p63) return 0;
    *out=(int64_t)n; return 1;
  }
  return 0;
}
static OValue ostd_time_duration_value(int64_t nanoseconds) {
  return (OValue){O_DURATION,0,{.i=nanoseconds}};
}
static OValue ostd_time_duration(OValue value) {
  if(value.kind==O_DURATION) return value;
  if(value.kind==O_QUANTITY) {
    if(value.as.quantity.unit.len!=6||memcmp(value.as.quantity.unit.data,"Second",6)!=0) return ofail("duration expects a duration, integer, or Second quantity");
    long double nanoseconds=(long double)value.as.quantity.magnitude*1000000000.0L;
    if(!isfinite(value.as.quantity.magnitude)||!isfinite(nanoseconds)||floorl(nanoseconds)!=nanoseconds||nanoseconds<-9223372036854775808.0L||nanoseconds>9223372036854775807.0L) return ofail("duration quantity must be an exact signed nanosecond count");
    return ostd_time_duration_value((int64_t)nanoseconds);
  }
  int64_t nanoseconds;
  if(!ointarg(value,&nanoseconds)) return ofail("duration expects a duration, integer, or Second quantity");
  return ostd_time_duration_value(nanoseconds);
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
static int oleapyear(int year) { return year%4==0&&(year%100!=0||year%400==0); }
static int odaysinmonth(int year, int month) {
  static const int lengths[12]={31,28,31,30,31,30,31,31,30,31,30,31};
  return month==2&&oleapyear(year)?29:lengths[month-1];
}
static int64_t odays_floor(int64_t seconds) {
  int64_t days=seconds/86400, remainder=seconds%86400;
  if(remainder<0) days--;
  return days;
}
static OValue ostd_time_datetime(int64_t seconds, int64_t nanosecond) {
  while(nanosecond<0) { if(seconds==INT64_MIN) return ofail("datetime is outside native range"); seconds--; nanosecond+=1000000000; }
  while(nanosecond>=1000000000) { if(seconds==INT64_MAX) return ofail("datetime is outside native range"); seconds++; nanosecond-=1000000000; }
  int year; unsigned month,day; ocivil_from_days(odays_floor(seconds),&year,&month,&day);
  if(year<1||year>9999) return ofail("datetime result is outside years 1..9999");
  return (OValue){O_DATETIME,0,{.datetime={seconds,(int32_t)nanosecond}}};
}
static OValue ostd_time_unix_parts(int64_t seconds, int64_t nanoseconds) {
  int64_t whole=nanoseconds/1000000000, remainder=nanoseconds%1000000000, normalized;
  if(__builtin_add_overflow(seconds,whole,&normalized)) return ofail("Unix timestamp is outside native range");
  if(remainder<0) {
    if(normalized==INT64_MIN) return ofail("Unix timestamp is outside native range");
    normalized--;
    remainder+=1000000000;
  }
  return ostd_time_datetime(normalized,remainder);
}
static OValue ostd_time_unix(const OValue *args, size_t count) {
  int64_t seconds,nanoseconds=0;
  if(count!=1&&count!=2) return ofail("unix expects seconds and optional nanoseconds");
  if(!ointarg(args[0],&seconds)||(count==2&&!ointarg(args[1],&nanoseconds))) return ofail("unix expects exact integer arguments");
  return ostd_time_unix_parts(seconds,nanoseconds);
}
static OValue ostd_time_unix_scaled(OValue value, int64_t scale) {
  int64_t count,total;
  if(!ointarg(value,&count)||__builtin_mul_overflow(count,scale,&total)) return ofail("scaled Unix timestamp is outside native range");
  return ostd_time_unix_parts(0,total);
}
static OValue ostd_time_date(const OValue *args, size_t count) {
  int64_t y,m,d,h,min,s,ns;
  if(count!=7&&count!=8) return ofail("date expects 7 fields and an optional location");
  if(!ointarg(args[0],&y)||!ointarg(args[1],&m)||!ointarg(args[2],&d)||!ointarg(args[3],&h)||!ointarg(args[4],&min)||!ointarg(args[5],&s)||!ointarg(args[6],&ns)) return ofail("date expects integer civil fields");
  if(count==8&&(args[7].kind!=O_STRING||args[7].as.s.len!=3||memcmp(args[7].as.s.data,"UTC",3)!=0)) return ofail("date location is not representable by the native UTC datetime ABI");
  if(y<1||y>9999||m<1||m>12||d<1||d>odaysinmonth((int)y,(int)m)||h<0||h>23||min<0||min>59||s<0||s>59||ns<0||ns>999999999) return ofail("invalid civil datetime");
  int64_t seconds=odays_from_civil((int)y,(unsigned)m,(unsigned)d)*86400+h*3600+min*60+s;
  return ostd_time_datetime(seconds,ns);
}
static OValue ostd_time_add_date(OValue date, OValue yearsValue, OValue monthsValue, OValue daysValue) {
  int64_t years,months,days;
  if(date.kind!=O_DATETIME||!ointarg(yearsValue,&years)||!ointarg(monthsValue,&months)||!ointarg(daysValue,&days)) return ofail("add_date expects datetime and integer offsets");
  int64_t dayNumber=odays_floor(date.as.datetime.seconds), dayStart;
  if(__builtin_mul_overflow(dayNumber,INT64_C(86400),&dayStart)) return ofail("add_date result is outside native range");
  int64_t timeOfDay=date.as.datetime.seconds-dayStart; int y; unsigned m,d; ocivil_from_days(dayNumber,&y,&m,&d);
  int64_t total, scaled;
  if(__builtin_mul_overflow(years,INT64_C(12),&scaled)||__builtin_add_overflow((int64_t)y*12+(int64_t)m-1,scaled,&total)||__builtin_add_overflow(total,months,&total)) return ofail("add_date result is outside native range");
  int64_t ny=total/12, monthIndex=total%12; if(monthIndex<0) { monthIndex+=12; ny--; }
  monthIndex++;
  if(ny<1||ny>9999) return ofail("add_date result is outside years 1..9999");
  int64_t targetDay;
  if(__builtin_add_overflow(odays_from_civil((int)ny,(unsigned)monthIndex,1),(int64_t)d-1,&targetDay)||__builtin_add_overflow(targetDay,days,&targetDay)) return ofail("add_date result is outside native range");
  int64_t seconds;
  if(__builtin_mul_overflow(targetDay,INT64_C(86400),&seconds)||__builtin_add_overflow(seconds,timeOfDay,&seconds)) return ofail("add_date result is outside native range");
  return ostd_time_datetime(seconds,date.as.datetime.nanosecond);
}
static OValue ostd_time_parse_duration(OValue value) {
  if(value.kind!=O_STRING) return ofail("parse_duration expects a string"); int64_t total=0,current=0; size_t i=0;
  if(value.as.s.len==0) return ofail("invalid duration");
  while(i<value.as.s.len) { if(value.as.s.data[i]<'0'||value.as.s.data[i]>'9') return ofail("invalid duration"); current=0; while(i<value.as.s.len&&value.as.s.data[i]>='0'&&value.as.s.data[i]<='9') { int digit=value.as.s.data[i++]-'0'; if(current>(INT64_MAX-digit)/10) return ofail("duration overflow"); current=current*10+digit; } if(i>=value.as.s.len) return ofail("invalid duration"); char unit=value.as.s.data[i++]; int64_t scale=unit=='h'?3600000000000LL:unit=='m'?60000000000LL:unit=='s'?1000000000LL:0,part; if(!scale||__builtin_mul_overflow(current,scale,&part)||__builtin_add_overflow(total,part,&total)) return ofail(scale?"duration overflow":"invalid duration"); }
  return (OValue){O_DURATION,0,{.i=total}};
}
static OValue ostd_time_add(OValue date, OValue duration) {
  if(date.kind!=O_DATETIME||duration.kind!=O_DURATION) return ofail("add expects datetime and duration without overflow");
  int64_t seconds=duration.as.i/1000000000, remainder=duration.as.i%1000000000, result;
  if(__builtin_add_overflow(date.as.datetime.seconds,seconds,&result)) return ofail("add expects datetime and duration without overflow");
  return ostd_time_datetime(result,(int64_t)date.as.datetime.nanosecond+remainder);
}
static OValue ostd_time_sub(OValue left, OValue right) {
  if(left.kind!=O_DATETIME||right.kind!=O_DATETIME) return ofail("sub expects datetimes without overflow");
  int64_t seconds,total;
  if(__builtin_sub_overflow(left.as.datetime.seconds,right.as.datetime.seconds,&seconds)||__builtin_mul_overflow(seconds,INT64_C(1000000000),&total)||__builtin_add_overflow(total,(int64_t)left.as.datetime.nanosecond-right.as.datetime.nanosecond,&total)) return ofail("sub expects datetimes without overflow");
  return (OValue){O_DURATION,0,{.i=total}};
}
typedef struct { char *data; size_t len, cap; } OTimeText;
static int otime_append(OTimeText *text, const char *data, size_t len) {
  if(len>SIZE_MAX-text->len-1) return 0;
  size_t needed=text->len+len+1;
  if(needed>text->cap) { size_t cap=text->cap?text->cap:64; while(cap<needed) { if(cap>SIZE_MAX/2) { cap=needed; break; } cap*=2; } char *next=(char *)realloc(text->data,cap); if(!next) return 0; text->data=next; text->cap=cap; }
  memcpy(text->data+text->len,data,len); text->len+=len; text->data[text->len]=0; return 1;
}
static int otime_appendf(OTimeText *text, const char *format, ...) {
  char buffer[64]; va_list args; va_start(args,format); int length=vsnprintf(buffer,sizeof(buffer),format,args); va_end(args);
  if(length<0) return 0; if((size_t)length<sizeof(buffer)) return otime_append(text,buffer,(size_t)length);
  char *large=(char *)malloc((size_t)length+1); if(!large) return 0; va_start(args,format); vsnprintf(large,(size_t)length+1,format,args); va_end(args); int ok=otime_append(text,large,(size_t)length); free(large); return ok;
}
static int otime_token(const char *layout, size_t len, size_t at, const char *token) { size_t tokenLen=strlen(token); return at+tokenLen<=len&&!memcmp(layout+at,token,tokenLen); }
static const char *otime_month_short(unsigned month) { static const char *names[]={"Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"}; return month>=1&&month<=12?names[month-1]:""; }
static const char *otime_month_long(unsigned month) { static const char *names[]={"January","February","March","April","May","June","July","August","September","October","November","December"}; return month>=1&&month<=12?names[month-1]:""; }
static const char *otime_weekday_short(unsigned weekday) { static const char *names[]={"Sun","Mon","Tue","Wed","Thu","Fri","Sat"}; return names[weekday%7]; }
static const char *otime_weekday_long(unsigned weekday) { static const char *names[]={"Sunday","Monday","Tuesday","Wednesday","Thursday","Friday","Saturday"}; return names[weekday%7]; }
static int otime_append_fraction(OTimeText *text, int32_t nanosecond, char separator, size_t digits, int trim) {
  char fraction[10]; snprintf(fraction,sizeof(fraction),"%09d",nanosecond); if(trim) while(digits>0&&fraction[digits-1]=='0') digits--; if(digits==0&&trim) return 1;
  if(!otime_append(text,&separator,1)) return 0; return otime_append(text,fraction,digits);
}
static OValue ostd_time_format(OValue date, OValue layout) {
  if(date.kind!=O_DATETIME||layout.kind!=O_STRING) return ofail("format expects datetime and layout strings");
  int64_t days=odays_floor(date.as.datetime.seconds), tod=date.as.datetime.seconds-days*86400; int year; unsigned month,day; ocivil_from_days(days,&year,&month,&day);
  unsigned hour=(unsigned)(tod/3600), minute=(unsigned)(tod/60%60), second=(unsigned)(tod%60); int weekday=(int)((days+4)%7); if(weekday<0) weekday+=7;
  OTimeText text={0}; size_t at=0;
  while(at<layout.as.s.len) {
    const char *source=layout.as.s.data;
    if(otime_token(source,layout.as.s.len,at,"January")) { if(!otime_append(&text,otime_month_long(month),strlen(otime_month_long(month)))) goto oom; at+=7; continue; }
    if(otime_token(source,layout.as.s.len,at,"Monday")) { if(!otime_append(&text,otime_weekday_long(weekday),strlen(otime_weekday_long(weekday)))) goto oom; at+=6; continue; }
    if(otime_token(source,layout.as.s.len,at,"2006")) { if(!otime_appendf(&text,"%04d",year)) goto oom; at+=4; continue; }
    if(otime_token(source,layout.as.s.len,at,"MST")) { if(!otime_append(&text,"UTC",3)) goto oom; at+=3; continue; }
    if(otime_token(source,layout.as.s.len,at,"Jan")) { if(!otime_append(&text,otime_month_short(month),3)) goto oom; at+=3; continue; }
    if(otime_token(source,layout.as.s.len,at,"Mon")) { if(!otime_append(&text,otime_weekday_short(weekday),3)) goto oom; at+=3; continue; }
    if(otime_token(source,layout.as.s.len,at,"Z07:00")) { if(!otime_append(&text,"Z",1)) goto oom; at+=6; continue; }
    if(otime_token(source,layout.as.s.len,at,"Z0700")) { if(!otime_append(&text,"Z",1)) goto oom; at+=5; continue; }
    if(otime_token(source,layout.as.s.len,at,"Z07")) { if(!otime_append(&text,"Z",1)) goto oom; at+=3; continue; }
    if(otime_token(source,layout.as.s.len,at,"-07:00")) { if(!otime_append(&text,"+00:00",6)) goto oom; at+=6; continue; }
    if(otime_token(source,layout.as.s.len,at,"-0700")) { if(!otime_append(&text,"+0000",5)) goto oom; at+=5; continue; }
    if(otime_token(source,layout.as.s.len,at,"-07")) { if(!otime_append(&text,"+00",3)) goto oom; at+=3; continue; }
    if(otime_token(source,layout.as.s.len,at,"2006")) { if(!otime_appendf(&text,"%04d",year)) goto oom; at+=4; continue; }
    if(otime_token(source,layout.as.s.len,at,"06")) { if(!otime_appendf(&text,"%02d",year%100)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"01")) { if(!otime_appendf(&text,"%02u",month)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"02")) { if(!otime_appendf(&text,"%02u",day)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"15")) { if(!otime_appendf(&text,"%02u",hour)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"03")) { unsigned value=hour%12; if(value==0) value=12; if(!otime_appendf(&text,"%02u",value)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"04")) { if(!otime_appendf(&text,"%02u",minute)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"05")) { if(!otime_appendf(&text,"%02u",second)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"PM")) { if(!otime_append(&text,hour>=12?"PM":"AM",2)) goto oom; at+=2; continue; }
    if(otime_token(source,layout.as.s.len,at,"pm")) { if(!otime_append(&text,hour>=12?"pm":"am",2)) goto oom; at+=2; continue; }
    if(source[at]=='_'&&at+1<layout.as.s.len&&source[at+1]=='2') { if(!otime_appendf(&text,"%2u",day)) goto oom; at+=2; continue; }
    if(source[at]=='.'||source[at]==',') { char separator=source[at]; size_t fractionAt=at+1; while(fractionAt<layout.as.s.len&&(source[fractionAt]=='0'||source[fractionAt]=='9')) fractionAt++; size_t digits=fractionAt-at-1; if(digits>=1&&digits<=9) { if(!otime_append_fraction(&text,date.as.datetime.nanosecond,separator,digits,source[at+1]=='9')) goto oom; at=fractionAt; continue; } }
    if(otime_token(source,layout.as.s.len,at,"1")) { if(!otime_appendf(&text,"%u",month)) goto oom; at++; continue; }
    if(otime_token(source,layout.as.s.len,at,"2")) { if(!otime_appendf(&text,"%u",day)) goto oom; at++; continue; }
    if(otime_token(source,layout.as.s.len,at,"3")) { unsigned value=hour%12; if(value==0) value=12; if(!otime_appendf(&text,"%u",value)) goto oom; at++; continue; }
    if(otime_token(source,layout.as.s.len,at,"4")) { if(!otime_appendf(&text,"%u",minute)) goto oom; at++; continue; }
    if(otime_token(source,layout.as.s.len,at,"5")) { if(!otime_appendf(&text,"%u",second)) goto oom; at++; continue; }
    if(!otime_append(&text,source+at,1)) goto oom; at++;
  }
  { OValue result=oownedstring(text.data,text.len); free(text.data); return result; }
oom: free(text.data); return ofail("out of memory");
}
static OValue ostd_time_datetime_text(OValue date) { return ostd_time_format(date,ostring("2006-01-02T15:04:05.999999999Z07:00")); }
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
  else if(ns>=1000ULL&&ns%1000ULL==0) at+=(size_t)snprintf(text+at,sizeof(text)-at,"%llu\xc2\xb5s",(unsigned long long)(ns/1000ULL));
  else at+=(size_t)snprintf(text+at,sizeof(text)-at,"%lluns",(unsigned long long)ns);
  int n=(int)at;
  return oownedstring(text,(size_t)n);
}
static void oprintdatetime(OValue date) {
  OValue text=ostd_time_datetime_text(date);
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
static int ocursoruvar(OCursor *cursor, uint64_t *out);
static int ocursortake(OCursor *cursor, size_t count, const unsigned char **out);
static int ocursorbyte(OCursor *cursor, unsigned char *out);
static int ocodecadd(OBuffer *buffer, const void *data, size_t len) {
  if(len>OCODEC_LIMIT||buffer->len>OCODEC_LIMIT-len) return 0;
  return obufferadd(buffer,data,len);
}
static int ocodecbyte(OBuffer *buffer, unsigned char value) { return ocodecadd(buffer,&value,1); }
static int ocodecuvar(OBuffer *buffer, uint64_t value) {
  unsigned char raw[10]; size_t count=0;
  while(value>=0x80) { raw[count++]=(unsigned char)(value|0x80); value>>=7; }
  raw[count++]=(unsigned char)value; return ocodecadd(buffer,raw,count);
}
static int ocodecvar(OBuffer *buffer, int64_t value) {
  uint64_t encoded=(uint64_t)value<<1; if(value<0) encoded=~encoded;
  return ocodecuvar(buffer,encoded);
}
static int ocodecu64be(OBuffer *buffer, uint64_t value) {
  unsigned char raw[8]; for(size_t i=0;i<8;i++) raw[i]=(unsigned char)(value>>(56-8*i));
  return ocodecadd(buffer,raw,8);
}
static int ocodecencodeinto(OBuffer *buffer, OValue value, unsigned depth);
static int ocodecdecodefrom(OCursor *cursor, OValue *out, unsigned depth);

static int ocodecstring(OBuffer *buffer, OString value) {
  return ocodecuvar(buffer,value.len)&&(!value.len||ocodecadd(buffer,value.data,value.len));
}
static int ocodecstringsame(OString value, const char *text) {
  size_t len=strlen(text); return value.len==len&&(!len||!memcmp(value.data,text,len));
}
static const char *ocodecseverity(int severity) {
  switch(severity) { case 1: return "info"; case 2: return "warning"; case 3: return "fatal"; default: return "error"; }
}
static int ocodecseverityvalue(OString value) {
  if(ocodecstringsame(value,"info")) return 1;
  if(ocodecstringsame(value,"warning")) return 2;
  if(ocodecstringsame(value,"fatal")) return 3;
  return 0;
}
static int ocodecownedstring(const unsigned char *data, size_t len, OString *out) {
  if(len>SIZE_MAX-1) return 0;
  char *copy=(char *)malloc(len+1); if(!copy) return 0;
  if(len) memcpy(copy,data,len); copy[len]=0;
  if(!otrack(copy)) { free(copy); return 0; }
  *out=(OString){copy,len}; return 1;
}
static int ocodecdecodeString(OCursor *cursor, OString *out) {
  uint64_t length; const unsigned char *data;
  if(!ocursoruvar(cursor,&length)||length>OCODEC_LIMIT||length>SIZE_MAX||!ocursortake(cursor,(size_t)length,&data)) return 0;
  return ocodecownedstring(data,(size_t)length,out);
}
static int ocodecstore(OValue value, OValue **out) {
  OValue *copy=(OValue *)malloc(sizeof(OValue)); if(!copy) return 0;
  *copy=value; if(!otrack(copy)) { free(copy); return 0; }
  *out=copy; return 1;
}
static int ocodecmap(OValue value, OBuffer *buffer, unsigned depth) {
  if(value.kind!=O_MAP||!value.as.map) return ocodecuvar(buffer,0);
  OMap *map=value.as.map; if(map->len>OCODEC_LIMIT||map->len>SIZE_MAX/sizeof(size_t)) return 0;
  size_t *order=map->len?(size_t *)malloc(map->len*sizeof(size_t)):NULL;
  if(map->len&&!order) return 0;
  for(size_t i=0;i<map->len;i++) order[i]=i;
  for(size_t i=1;i<map->len;i++) {
    size_t item=order[i],at=i;
    while(at>0) {
      OString left=map->entries[order[at-1]].key,right=map->entries[item].key;
      size_t common=left.len<right.len?left.len:right.len;
      int compare=common?memcmp(left.data,right.data,common):0;
      if(compare<0||(compare==0&&left.len<=right.len)) break;
      order[at]=order[at-1]; at--;
    }
    order[at]=item;
  }
  int ok=ocodecuvar(buffer,map->len);
  for(size_t i=0;ok&&i<map->len;i++) { OMapEntry entry=map->entries[order[i]]; ok=ocodecstring(buffer,entry.key)&&ocodecencodeinto(buffer,entry.value,depth+1); }
  free(order); return ok;
}
static OValue *ocodecmapfield(OValue frame, const char *name) {
  return frame.kind==O_MAP&&frame.as.map?omapget(frame,name):NULL;
}
static int ocodecframeuint(OValue frame, const char *name, OBuffer *buffer) {
  OValue *value=ocodecmapfield(frame,name); if(!value||value->kind!=O_UINT) return 0;
  return ocodecuvar(buffer,value->as.u);
}
static int ocodecstack(OValue value, OBuffer *buffer, unsigned depth) {
  if(value.kind!=O_LIST) return ocodecuvar(buffer,0);
  if(value.as.list.len>OCODEC_LIMIT) return 0;
  if(!ocodecuvar(buffer,value.as.list.len)) return 0;
  for(size_t i=0;i<value.as.list.len;i++) {
    OValue frame=value.as.list.items[i]; OValue *name=ocodecmapfield(frame,"name"),*source=ocodecmapfield(frame,"source");
    if(!name||!source||name->kind!=O_STRING||source->kind!=O_STRING||!ocodecstring(buffer,name->as.s)||!ocodecstring(buffer,source->as.s)) return 0;
    if(!ocodecframeuint(frame,"start_line",buffer)||!ocodecframeuint(frame,"start_col",buffer)||!ocodecframeuint(frame,"end_line",buffer)||!ocodecframeuint(frame,"end_col",buffer)) return 0;
  }
  (void)depth; return 1;
}
static int ocodecerror(OBuffer *buffer, OValue value, unsigned depth) {
  if(value.kind!=O_ERROR||depth>OCODEC_DEPTH) return 0;
  OError error=value.as.error; OString kind=error.kind;
  if(kind.len==0) kind=(OString){"unknown_error",13};
  if(!ocodecbyte(buffer,0x0a)||!ocodecuvar(buffer,1)||!ocodecstring(buffer,kind)||!ocodecstring(buffer,kind)||!ocodecstring(buffer,error.message)||!ocodecstring(buffer,error.user_message)||!ocodecstring(buffer,error.operation)||!ocodecstring(buffer,error.resource)) return 0;
  if(!ocodecbyte(buffer,error.retryable?1:0)||(error.retryable&&!ocodecbyte(buffer,1))||!ocodecstring(buffer,(OString){ocodecseverity(error.severity),strlen(ocodecseverity(error.severity))})||!ocodecstring(buffer,(OString){NULL,0})||!ocodecstring(buffer,(OString){NULL,0})||!ocodecstring(buffer,(OString){NULL,0})) return 0;
  if(!ocodecbyte(buffer,0)) return 0;
  if(error.source.len) {
    if(!ocodecbyte(buffer,1)||!ocodecstring(buffer,error.source)||!ocodecuvar(buffer,0)||!ocodecuvar(buffer,0)||!ocodecuvar(buffer,0)||!ocodecuvar(buffer,0)) return 0;
  } else if(!ocodecbyte(buffer,0)) return 0;
  if(error.cause) { if(error.cause->kind!=O_ERROR||!ocodecbyte(buffer,1)||!ocodecerror(buffer,*error.cause,depth+1)) return 0; }
  else if(!ocodecbyte(buffer,0)) return 0;
  if(!ocodecstack(error.stack?*error.stack:(OValue){O_LIST,0,{.list={NULL,0}}},buffer,depth)) return 0;
  if(!ocodecmap(error.metadata?*error.metadata:(OValue){O_MAP,0,{.map=NULL}},buffer,depth)||!ocodecmap(error.debug?*error.debug:(OValue){O_MAP,0,{.map=NULL}},buffer,depth)) return 0;
  return 1;
}
static int ocodecmapdecode(OCursor *cursor, OValue *out, unsigned depth) {
  uint64_t count; if(depth>OCODEC_DEPTH||!ocursoruvar(cursor,&count)||count>OCODEC_LIMIT/sizeof(OMapEntry)||count>cursor->len-cursor->off) return 0;
  OMap *map=(OMap *)calloc(1,sizeof(OMap)); if(!map) return 0;
  OMapEntry *entries=count?(OMapEntry *)calloc((size_t)count,sizeof(OMapEntry)):NULL;
  if(count&&!entries) { free(map); return 0; }
  if(!otrack(map)||!otrack(entries)) { free(map); free(entries); return 0; }
  map->entries=entries; map->len=(size_t)count;
  for(size_t i=0;i<map->len;i++) { if(!ocodecdecodeString(cursor,&map->entries[i].key)||!ocodecdecodefrom(cursor,&map->entries[i].value,depth+1)) return 0; }
  *out=(OValue){O_MAP,0,{.map=map}}; return 1;
}
static int ocodecmapnew(const OString *keys, const OValue *values, size_t count, OValue *out) {
  if(count>OCODEC_LIMIT/sizeof(OMapEntry)) return 0;
  OMap *map=(OMap *)calloc(1,sizeof(OMap)); if(!map) return 0;
  OMapEntry *entries=count?(OMapEntry *)calloc(count,sizeof(OMapEntry)):NULL;
  if(count&&!entries) { free(map); return 0; }
  if(!otrack(map)||!otrack(entries)) { free(map); free(entries); return 0; }
  map->entries=entries; map->len=count; for(size_t i=0;i<count;i++) { entries[i].key=keys[i]; entries[i].value=values[i]; }
  *out=(OValue){O_MAP,0,{.map=map}}; return 1;
}
static int ocodecstackdecode(OCursor *cursor, OValue *out, unsigned depth) {
  uint64_t count; if(depth>OCODEC_DEPTH||!ocursoruvar(cursor,&count)||count>OCODEC_LIMIT/sizeof(OValue)||count>cursor->len-cursor->off) return 0;
  OValue *frames=count?(OValue *)calloc((size_t)count,sizeof(OValue)):NULL; if(count&&!frames) return 0;
  if(count&&!otrack(frames)) { free(frames); return 0; }
  const char *names[6]={"name","source","start_line","start_col","end_line","end_col"};
  for(size_t i=0;i<(size_t)count;i++) {
    OString text[2]; uint64_t line[4]; OValue values[6];
    if(!ocodecdecodeString(cursor,&text[0])||!ocodecdecodeString(cursor,&text[1])) return 0;
    for(size_t field=0;field<4;field++) if(!ocursoruvar(cursor,&line[field])) return 0;
    values[0]=(OValue){O_STRING,0,{.s=text[0]}}; values[1]=(OValue){O_STRING,0,{.s=text[1]}};
    for(size_t field=0;field<4;field++) values[field+2]=ouint(line[field]);
    OString keys[6]={{names[0],4},{names[1],6},{names[2],10},{names[3],9},{names[4],8},{names[5],7}};
    if(!ocodecmapnew(keys,values,6,&frames[i])) return 0;
  }
  *out=olist(frames,(size_t)count); return !ohaderror;
}
static int ocodecstringvalidtimestamp(OString value) {
  if(value.len<20||value.len>40||value.data[4]!='-'||value.data[7]!='-'||value.data[10]!='T'||value.data[13]!=':'||value.data[16]!=':') return 0;
  for(size_t i=0;i<19;i++) if(i!=4&&i!=7&&i!=10&&i!=13&&i!=16&&(value.data[i]<'0'||value.data[i]>'9')) return 0;
  size_t at=19;
  if(at<value.len&&value.data[at]=='.') {
    size_t fractionStart=++at;
    while(at<value.len&&value.data[at]>='0'&&value.data[at]<='9') at++;
    if(at==fractionStart||at-fractionStart>9) return 0;
  }
  if(at+1==value.len&&value.data[at]=='Z') return 1;
  return at+6==value.len&&(value.data[at]=='+'||value.data[at]=='-')&&
    value.data[at+3]==':'&&value.data[at+1]>='0'&&value.data[at+1]<='9'&&
    value.data[at+2]>='0'&&value.data[at+2]<='9'&&value.data[at+4]>='0'&&
    value.data[at+4]<='9'&&value.data[at+5]>='0'&&value.data[at+5]<='9';
}
static int ocodecerrorDecode(OCursor *cursor, OValue *out, unsigned depth) {
  if(depth>OCODEC_DEPTH) return 0;
  uint64_t version; OString kind,code,message,user,operation,resource,severity,correlation,component,remediation;
  if(!ocursoruvar(cursor,&version)||version!=1||!ocodecdecodeString(cursor,&kind)||!ocodecdecodeString(cursor,&code)||!ocodecdecodeString(cursor,&message)||!ocodecdecodeString(cursor,&user)||!ocodecdecodeString(cursor,&operation)||!ocodecdecodeString(cursor,&resource)) return 0;
  unsigned char hasRetryable,retryable=0; if(!ocursorbyte(cursor,&hasRetryable)||(hasRetryable>1)||(hasRetryable&&!ocursorbyte(cursor,&retryable))) return 0;
  if(!ocodecdecodeString(cursor,&severity)||!ocodecdecodeString(cursor,&correlation)||!ocodecdecodeString(cursor,&component)||!ocodecdecodeString(cursor,&remediation)) return 0;
  unsigned char hasTimestamp; OString timestamp; if(!ocursorbyte(cursor,&hasTimestamp)||(hasTimestamp>1)||(hasTimestamp&&!ocodecdecodeString(cursor,&timestamp))||(hasTimestamp&&!ocodecstringvalidtimestamp(timestamp))) return 0;
  unsigned char hasLocation; OString source={0}; uint64_t line[4]; if(!ocursorbyte(cursor,&hasLocation)||(hasLocation>1)) return 0;
  if(hasLocation) { if(!ocodecdecodeString(cursor,&source)) return 0; for(size_t i=0;i<4;i++) if(!ocursoruvar(cursor,&line[i])) return 0; }
  unsigned char hasCause; if(!ocursorbyte(cursor,&hasCause)||(hasCause>1)) return 0;
  OValue causeValue; OValue *cause=NULL; if(hasCause) { if(!ocodecdecodefrom(cursor,&causeValue,depth+1)||causeValue.kind!=O_ERROR||!ocodecstore(causeValue,&cause)) return 0; }
  OValue stack,metadata,debug; if(!ocodecstackdecode(cursor,&stack,depth+1)||!ocodecmapdecode(cursor,&metadata,depth+1)||!ocodecmapdecode(cursor,&debug,depth+1)) return 0;
  if(kind.len==0) kind=(OString){"unknown_error",13};
  *out=(OValue){O_ERROR,0,{.error={.kind=kind,.message=message,.user_message=user,.operation=operation,.resource=resource,.source=source,.location=source,.cause=cause,.stack=NULL,.metadata=NULL,.debug=NULL,.retryable=(hasRetryable&&retryable)!=0,.severity=ocodecseverityvalue(severity)}}};
  if(!ocodecstore(stack,&out->as.error.stack)||!ocodecstore(metadata,&out->as.error.metadata)||!ocodecstore(debug,&out->as.error.debug)) return 0;
  (void)code; (void)correlation; (void)component; (void)remediation; (void)line;
  return 1;
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
    return ocodecbyte(buffer,0x1c)&&ocodecvar(buffer,value.as.datetime.seconds)&&ocodecuvar(buffer,(uint64_t)value.as.datetime.nanosecond)&&ocodecbyte(buffer,0)&&ocodecuvar(buffer,3)&&obuffertext(buffer,"UTC");
  }
  case O_DURATION:
    return ocodecbyte(buffer,0x1a)&&ocodecvar(buffer,value.as.i);
  case O_ERROR:
    return ocodecerror(buffer,value,depth);
  case O_RESULT:
    if(!ocodecbyte(buffer,0x16)) return 0;
    if(!value.as.box.value||!value.fbits) {
      OValue error=oerrorvalue("unknown_error","");
      if(value.as.box.value&&value.as.box.value->kind==O_ERROR) error=*value.as.box.value;
      return ocodecbyte(buffer,0)&&ocodecerror(buffer,error,depth+1);
    }
    return ocodecbyte(buffer,1)&&ocodecencodeinto(buffer,*value.as.box.value,depth+1);
  case O_OPTION:
    if(!ocodecbyte(buffer,0x17)) return 0;
    if(!value.fbits) return ocodecbyte(buffer,0);
    return value.as.box.value&&ocodecbyte(buffer,1)&&ocodecencodeinto(buffer,*value.as.box.value,depth+1);
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
  case 0x0a:
    return ocodecerrorDecode(cursor,out,depth);
  case 0x16: {
    unsigned char present;
    if(!ocursorbyte(cursor,&present)||present>1) return 0;
    if(present) {
      OValue value; if(!ocodecdecodefrom(cursor,&value,depth+1)) return 0;
      *out=oresultok(value); return !ohaderror;
    }
    OValue error; if(!ocodecdecodefrom(cursor,&error,depth+1)||error.kind!=O_ERROR) return 0;
    *out=oresultfail(error); return !ohaderror;
  }
  case 0x17: {
    unsigned char present;
    if(!ocursorbyte(cursor,&present)||present>1) return 0;
    if(!present) { *out=ooptionNone(); return !ohaderror; }
    OValue value; if(!ocodecdecodefrom(cursor,&value,depth+1)) return 0;
    *out=ooptionSome(value); return !ohaderror;
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
    if(nanos>999999999) return 0;
    *out=(OValue){O_DATETIME,0,{.datetime={seconds,(int32_t)nanos}}}; return 1;
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
  case O_LIST: {
    int tuple=value.fbits!=0;
    if(!obuffertext(buffer,tuple?"(":"[")) return 0;
    for(size_t i=0;i<value.as.list.len;i++) { if(i&&!obuffertext(buffer,", ")) return 0; if(!orender(buffer,value.as.list.items[i])) return 0; }
    if(tuple&&value.as.list.len==1&&!obuffertext(buffer,",")) return 0;
    return obuffertext(buffer,tuple?")":"]");
  }
  case O_ENUM:
    if(!obufferadd(buffer,value.as.en.type_name.data,value.as.en.type_name.len)||!obuffertext(buffer,".")||!obufferadd(buffer,value.as.en.case_name.data,value.as.en.case_name.len)) return 0;
    if(value.as.en.payload.len) { if(!obuffertext(buffer,"(")) return 0; for(size_t i=0;i<value.as.en.payload.len;i++) { if(i&&!obuffertext(buffer,", ")) return 0; if(!orender(buffer,value.as.en.payload.items[i])) return 0; } if(!obuffertext(buffer,")")) return 0; } return 1;
  case O_COMPLEX:
    n=ofloattext(text,sizeof(text),value.as.cx.real); if(!obufferadd(buffer,text,n)) return 0; if(value.as.cx.imag>=0&&!obuffertext(buffer,"+")) return 0; n=ofloattext(text,sizeof(text),value.as.cx.imag); return obufferadd(buffer,text,n)&&obuffertext(buffer,"i");
  case O_DATETIME: {
    OValue rendered=ostd_time_datetime_text(value); if(rendered.kind!=O_STRING) return 0;
    n=(size_t)snprintf(text,sizeof(text),"datetime(\"%.*s\")",(int)rendered.as.s.len,rendered.as.s.data); return obufferadd(buffer,text,n);
  }
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
static OValue ocrypto_mix(const OValue *values, size_t count) {
  uint64_t state=UINT64_C(0xcbf29ce484222325);
  for(size_t i=0;i<count;i++) {
    OBuffer rendered={0};
    if(!orender(&rendered,values[i])) { free(rendered.data); return ofail("mix cannot render native value"); }
    for(size_t j=0;j<rendered.len;j++) {
      state^=(uint64_t)rendered.data[j];
      state*=UINT64_C(0x100000001b3);
      state=orol64(state,13)^(state>>7);
    }
    free(rendered.data);
    state^=UINT64_C(0xff);
    state=orol64(state,17)+UINT64_C(0x9e3779b97f4a7c15);
  }
  char hex[17];
  snprintf(hex,sizeof(hex),"%016llx",(unsigned long long)state);
  return oownedstring(hex,16);
}
static uint64_t ocrypto_noise_hash(const unsigned char *data, size_t len) {
  uint64_t hash=UINT64_C(0xcbf29ce484222325);
  for(size_t i=0;i<len;i++) { hash^=(uint64_t)data[i]; hash*=UINT64_C(0x100000001b3); hash=orol64(hash,11)^(hash>>5); }
  return hash;
}
static OValue ocrypto_noise(const OValue *values, size_t count) {
  uint64_t accumulator=UINT64_C(0x6a09e667f3bcc909);
  for(size_t i=0;i<count;i++) {
    OBuffer rendered={0};
    if(!orender(&rendered,values[i])) { free(rendered.data); return ofail("noise cannot render native value"); }
    uint64_t hash=ocrypto_noise_hash((const unsigned char *)rendered.data,rendered.len);
    free(rendered.data);
    accumulator^=hash+(uint64_t)i*UINT64_C(0x9e3779b97f4a7c15);
    accumulator=orol64(accumulator,27)^(accumulator<<7);
  }
  char hex[17];
  snprintf(hex,sizeof(hex),"%016llx",(unsigned long long)accumulator);
  return oownedstring(hex,16);
}
`
