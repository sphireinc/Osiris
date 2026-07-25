package osiris

// cRuntimeComplexTensorComponent is appended after the stable scalar runtime.
// Its position preserves definitions that wrap the f64 tensor and linalg helpers.
const cRuntimeComplexTensorComponent = `static OValue oat(OValue x, OValue index) {
  if(x.kind!=O_COMPLEX_TENSOR || index.kind!=O_LIST) return oat_real(x,index);
  size_t count=index.as.list.len;
  if(count==0 || count>x.as.ctensor.rank) return ofail("tensor coordinate rank does not match tensor rank");
  size_t offset=0, stride=1;
  for(size_t axis=x.as.ctensor.rank;axis>count;axis--) stride*=x.as.ctensor.shape[axis-1];
  size_t block=stride;
  for(size_t axis=count;axis>0;axis--) { size_t at=axis-1, coordinate; if(!odimension(index.as.list.items[at],&coordinate)||coordinate>=x.as.ctensor.shape[at]) return ofail("tensor coordinate out of range"); offset+=coordinate*stride; stride*=x.as.ctensor.shape[at]; }
  if(count==x.as.ctensor.rank) return ocomplex(x.as.ctensor.data[offset].real,x.as.ctensor.data[offset].imag);
  OComplex *data=block?(OComplex *)malloc(block*sizeof(OComplex)):NULL;
  if(block&&!data) return ofail("out of memory");
  if(block) memcpy(data,x.as.ctensor.data+offset,block*sizeof(OComplex));
  return ocomplextensordata(data,block,x.as.ctensor.shape+count,x.as.ctensor.rank-count);
}
static OValue oreshape(OValue tensor, OValue shape_arg) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return oreshape_real(tensor,shape_arg);
  size_t *shape,n,rank;
  if(!oshapeargs(&shape_arg,1,&shape,&rank,&n)) return ofail("reshape expects non-negative integer dimensions");
  if(n!=tensor.as.ctensor.len) { free(shape); return ofail("tensor shape does not match data"); }
  OComplex *data=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL;
  if(n&&!data) { free(shape); return ofail("out of memory"); }
  if(n) memcpy(data,tensor.as.ctensor.data,n*sizeof(OComplex));
  OValue out=ocomplextensordata(data,n,shape,rank);
  free(shape);
  return out;
}
static OValue osetat(OValue tensor, OValue index, OValue value) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return osetat_real(tensor,index,value);
  size_t offset, block=1, coordinate_rank=tensor.as.ctensor.rank;
  if(index.kind==O_LIST) {
    coordinate_rank=index.as.list.len;
    if(coordinate_rank==0||coordinate_rank>tensor.as.ctensor.rank) return ofail("tensor coordinate rank does not match tensor rank");
    size_t stride=1;
    for(size_t axis=tensor.as.ctensor.rank;axis>coordinate_rank;axis--) stride*=tensor.as.ctensor.shape[axis-1];
    block=stride; offset=0;
    for(size_t axis=coordinate_rank;axis>0;axis--) { size_t at=axis-1, coordinate; if(!odimension(index.as.list.items[at],&coordinate)||coordinate>=tensor.as.ctensor.shape[at]) return ofail("tensor coordinate out of range"); offset+=coordinate*stride; stride*=tensor.as.ctensor.shape[at]; }
  } else { if(!odimension(index,&offset)||offset>=tensor.as.ctensor.len) return ofail("setat index out of range"); }
  size_t n=tensor.as.ctensor.len;
  OComplex *data=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL;
  if(n&&!data) return ofail("out of memory");
  if(n) memcpy(data,tensor.as.ctensor.data,n*sizeof(OComplex));
  if(index.kind==O_LIST&&coordinate_rank<tensor.as.ctensor.rank) {
    if(value.kind!=O_COMPLEX_TENSOR||value.as.ctensor.rank!=tensor.as.ctensor.rank-coordinate_rank) { free(data); return ofail("tensor replacement shape does not match coordinate slice"); }
    for(size_t i=0;i<value.as.ctensor.rank;i++) if(value.as.ctensor.shape[i]!=tensor.as.ctensor.shape[coordinate_rank+i]) { free(data); return ofail("tensor replacement shape does not match coordinate slice"); }
    if(block) memcpy(data+offset,value.as.ctensor.data,block*sizeof(OComplex));
  } else {
    OComplex replacement;
    if(!ocomplexnum(value,&replacement)) { free(data); return ofail("tensor replacement must be numeric"); }
    data[offset]=replacement;
  }
  return ocomplextensordata(data,n,tensor.as.ctensor.shape,tensor.as.ctensor.rank);
}
static OValue obin(const char *op, OValue a, OValue b) {
  if(a.kind!=O_COMPLEX_TENSOR&&b.kind!=O_COMPLEX_TENSOR) return obin_real(op,a,b);
  if(strcmp(op,"+")&&strcmp(op,"-")&&strcmp(op,"*")&&strcmp(op,"/")) return obin_real(op,a,b);
  size_t arank=a.kind==O_COMPLEX_TENSOR?a.as.ctensor.rank:(a.kind==O_TENSOR?a.as.tensor.rank:0);
  size_t brank=b.kind==O_COMPLEX_TENSOR?b.as.ctensor.rank:(b.kind==O_TENSOR?b.as.tensor.rank:0);
  size_t rank=arank>brank?arank:brank,total=1;
  size_t *shape=rank?(size_t *)malloc(rank*sizeof(size_t)):NULL;
  if(rank&&!shape) return ofail("out of memory");
  for(size_t axis=0;axis<rank;axis++) { size_t adim=1,bdim=1; if(axis>=rank-arank) adim=a.kind==O_COMPLEX_TENSOR?a.as.ctensor.shape[axis-(rank-arank)]:a.as.tensor.shape[axis-(rank-arank)]; if(axis>=rank-brank) bdim=b.kind==O_COMPLEX_TENSOR?b.as.ctensor.shape[axis-(rank-brank)]:b.as.tensor.shape[axis-(rank-brank)]; if(adim!=bdim&&adim!=1&&bdim!=1) { free(shape); return ofail("tensor shape mismatch"); } shape[axis]=adim>bdim?adim:bdim; if(shape[axis]&&total>SIZE_MAX/shape[axis]) { free(shape); return ofail("tensor is too large"); } total*=shape[axis]; }
  OComplex *data=total?(OComplex *)malloc(total*sizeof(OComplex)):NULL;
  if(total&&!data) { free(shape); return ofail("out of memory"); }
  for(size_t output=0;output<total;output++) {
    size_t rest=output,ai=0,bi=0;
    for(size_t axis=rank;axis>0;axis--) {
      size_t at=axis-1, coordinate=rest%shape[at]; rest/=shape[at];
      if(at>=rank-arank) { size_t input=at-(rank-arank), dim=a.kind==O_COMPLEX_TENSOR?a.as.ctensor.shape[input]:a.as.tensor.shape[input], stride=1; for(size_t next=input+1;next<arank;next++) stride*=a.kind==O_COMPLEX_TENSOR?a.as.ctensor.shape[next]:a.as.tensor.shape[next]; if(dim!=1) ai+=coordinate*stride; }
      if(at>=rank-brank) { size_t input=at-(rank-brank), dim=b.kind==O_COMPLEX_TENSOR?b.as.ctensor.shape[input]:b.as.tensor.shape[input], stride=1; for(size_t next=input+1;next<brank;next++) stride*=b.kind==O_COMPLEX_TENSOR?b.as.ctensor.shape[next]:b.as.tensor.shape[next]; if(dim!=1) bi+=coordinate*stride; }
    }
    OComplex x,y;
    if(a.kind==O_COMPLEX_TENSOR) x=a.as.ctensor.data[ai]; else if(a.kind==O_TENSOR) x=(OComplex){a.as.tensor.data[ai],0}; else if(!ocomplexnum(a,&x)) { free(shape); free(data); return ofail("tensor scalar must be numeric"); }
    if(b.kind==O_COMPLEX_TENSOR) y=b.as.ctensor.data[bi]; else if(b.kind==O_TENSOR) y=(OComplex){b.as.tensor.data[bi],0}; else if(!ocomplexnum(b,&y)) { free(shape); free(data); return ofail("tensor scalar must be numeric"); }
    if(!strcmp(op,"+")) data[output]=(OComplex){x.real+y.real,x.imag+y.imag}; else if(!strcmp(op,"-")) data[output]=(OComplex){x.real-y.real,x.imag-y.imag}; else if(!strcmp(op,"*")) data[output]=(OComplex){x.real*y.real-x.imag*y.imag,x.real*y.imag+x.imag*y.real}; else { double scale=y.real*y.real+y.imag*y.imag; if(scale==0) { free(shape); free(data); return ofail("division by zero"); } data[output]=(OComplex){(x.real*y.real+x.imag*y.imag)/scale,(x.imag*y.real-x.real*y.imag)/scale}; }
  }
  OValue out=ocomplextensordata(data,total,shape,rank); free(shape); return out;
}
static OValue otake(OValue tensor, OValue count_arg) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return otake_real(tensor,count_arg);
  size_t count;
  if(!odimension(count_arg,&count)) return ofail("take count must be non-negative");
  if(count>tensor.as.ctensor.len) count=tensor.as.ctensor.len;
  OComplex *data=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL;
  if(count&&!data) return ofail("out of memory");
  if(count) memcpy(data,tensor.as.ctensor.data,count*sizeof(OComplex));
  size_t shape[1]={count};
  return ocomplextensordata(data,count,shape,1);
}
static OValue oslice(OValue tensor, OValue start_arg, OValue end_arg) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return oslice_real(tensor,start_arg,end_arg);
  if(tensor.as.ctensor.rank==0) return ofail("slice expects a tensor with a leading dimension");
  size_t start,end;
  if(!odimension(start_arg,&start)) return ofail("slice start must be a non-negative integer");
  if(!odimension(end_arg,&end)) return ofail("slice end must be a non-negative integer");
  if(start>end) return ofail("slice end must not precede start");
  size_t limit=tensor.as.ctensor.shape[0];
  if(start>limit) start=limit;
  if(end>limit) end=limit;
  size_t block=1;
  for(size_t axis=tensor.as.ctensor.rank;axis>1;axis--) block*=tensor.as.ctensor.shape[axis-1];
  size_t count=(end-start)*block;
  OComplex *data=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL;
  if(count&&!data) return ofail("out of memory");
  if(count) memcpy(data,tensor.as.ctensor.data+start*block,count*sizeof(OComplex));
  size_t *shape=(size_t *)malloc(tensor.as.ctensor.rank*sizeof(size_t));
  if(!shape) { free(data); return ofail("out of memory"); }
  memcpy(shape,tensor.as.ctensor.shape,tensor.as.ctensor.rank*sizeof(size_t));
  shape[0]=end-start;
  OValue out=ocomplextensordata(data,count,shape,tensor.as.ctensor.rank);
  free(shape);
  return out;
}
static OValue osliceaxis(OValue tensor, OValue start_arg, OValue end_arg, OValue axis_arg) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return osliceaxis_real(tensor,start_arg,end_arg,axis_arg);
  if(tensor.as.ctensor.rank==0) return ofail("slice expects a tensor with a leading dimension");
  size_t start,end,axis;
  if(!odimension(start_arg,&start)) return ofail("slice start must be a non-negative integer");
  if(!odimension(end_arg,&end)) return ofail("slice end must be a non-negative integer");
  if(!odimension(axis_arg,&axis)||axis>=tensor.as.ctensor.rank) return ofail("slice axis out of range");
  if(start>end) return ofail("slice end must not precede start");
  size_t limit=tensor.as.ctensor.shape[axis];
  if(start>limit) start=limit;
  if(end>limit) end=limit;
  size_t outer=1,inner=1;
  for(size_t i=0;i<axis;i++) outer*=tensor.as.ctensor.shape[i];
  for(size_t i=axis+1;i<tensor.as.ctensor.rank;i++) inner*=tensor.as.ctensor.shape[i];
  size_t span=end-start,count=outer*span*inner;
  OComplex *data=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL;
  if(count&&!data) return ofail("out of memory");
  for(size_t group=0;group<outer;group++) {
    size_t source=(group*limit+start)*inner,destination=group*span*inner;
    if(span&&inner) memcpy(data+destination,tensor.as.ctensor.data+source,span*inner*sizeof(OComplex));
  }
  size_t *shape=(size_t *)malloc(tensor.as.ctensor.rank*sizeof(size_t));
  if(!shape) { free(data); return ofail("out of memory"); }
  memcpy(shape,tensor.as.ctensor.shape,tensor.as.ctensor.rank*sizeof(size_t));
  shape[axis]=span;
  OValue out=ocomplextensordata(data,count,shape,tensor.as.ctensor.rank);
  free(shape);
  return out;
}
static OValue otranspose(OValue tensor) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return otranspose_real(tensor);
  if(tensor.as.ctensor.rank!=2) return ofail("transpose expects a 2D matrix");
  size_t rows=tensor.as.ctensor.shape[0],cols=tensor.as.ctensor.shape[1],n=tensor.as.ctensor.len;
  OComplex *data=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL;
  if(n&&!data) return ofail("out of memory");
  for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) data[col*rows+row]=tensor.as.ctensor.data[row*cols+col];
  size_t shape[2]={cols,rows};
  return ocomplextensordata(data,n,shape,2);
}
static OValue omatmul(OValue left, OValue right) {
  if(left.kind!=O_COMPLEX_TENSOR&&right.kind!=O_COMPLEX_TENSOR) return omatmul_real(left,right);
  size_t lrank=left.kind==O_COMPLEX_TENSOR?left.as.ctensor.rank:(left.kind==O_TENSOR?left.as.tensor.rank:0);
  size_t rrank=right.kind==O_COMPLEX_TENSOR?right.as.ctensor.rank:(right.kind==O_TENSOR?right.as.tensor.rank:0);
  if(lrank!=2||rrank!=2) return ofail("matmul expects 2D matrices");
  size_t rows=left.kind==O_COMPLEX_TENSOR?left.as.ctensor.shape[0]:left.as.tensor.shape[0],mid=left.kind==O_COMPLEX_TENSOR?left.as.ctensor.shape[1]:left.as.tensor.shape[1],right_rows=right.kind==O_COMPLEX_TENSOR?right.as.ctensor.shape[0]:right.as.tensor.shape[0],cols=right.kind==O_COMPLEX_TENSOR?right.as.ctensor.shape[1]:right.as.tensor.shape[1];
  if(mid!=right_rows) return ofail("matmul shape mismatch");
  if(rows&&cols>SIZE_MAX/rows) return ofail("tensor is too large");
  size_t n=rows*cols;
  OComplex *data=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL;
  if(n&&!data) return ofail("out of memory");
  for(size_t row=0;row<rows;row++) for(size_t col=0;col<cols;col++) {
    OComplex total={0,0};
    for(size_t k=0;k<mid;k++) {
      OComplex x=left.kind==O_COMPLEX_TENSOR?left.as.ctensor.data[row*mid+k]:(OComplex){left.as.tensor.data[row*mid+k],0};
      OComplex y=right.kind==O_COMPLEX_TENSOR?right.as.ctensor.data[k*cols+col]:(OComplex){right.as.tensor.data[k*cols+col],0};
      total.real+=x.real*y.real-x.imag*y.imag;
      total.imag+=x.real*y.imag+x.imag*y.real;
    }
    data[row*cols+col]=total;
  }
  size_t shape[2]={rows,cols};
  return ocomplextensordata(data,n,shape,2);
}
static OComplex ocadd(OComplex left, OComplex right) { return (OComplex){left.real+right.real,left.imag+right.imag}; }
static OComplex ocsub(OComplex left, OComplex right) { return (OComplex){left.real-right.real,left.imag-right.imag}; }
static OComplex ocmul(OComplex left, OComplex right) { return (OComplex){left.real*right.real-left.imag*right.imag,left.real*right.imag+left.imag*right.real}; }
static OComplex ocdiv(OComplex left, OComplex right) { double scale=right.real*right.real+right.imag*right.imag; return (OComplex){(left.real*right.real+left.imag*right.imag)/scale,(left.imag*right.real-left.real*right.imag)/scale}; }
static OValue otrace(OValue tensor) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return otrace_real(tensor);
  if(tensor.as.ctensor.rank!=2) return ofail("trace expects a matrix");
  size_t rows=tensor.as.ctensor.shape[0],cols=tensor.as.ctensor.shape[1],limit=rows<cols?rows:cols;
  OComplex total={0,0};
  for(size_t i=0;i<limit;i++) total=ocadd(total,tensor.as.ctensor.data[i*cols+i]);
  return ocomplex(total.real,total.imag);
}
static OValue odet(OValue tensor) {
  if(tensor.kind!=O_COMPLEX_TENSOR) return odet_real(tensor);
  if(tensor.as.ctensor.rank!=2||tensor.as.ctensor.shape[0]!=tensor.as.ctensor.shape[1]) return ofail("det expects a square matrix");
  size_t n=tensor.as.ctensor.shape[0];
  if(n==0) return ocomplex(1,0);
  if(n>SIZE_MAX/n||n*n>SIZE_MAX/sizeof(OComplex)) return ofail("matrix is too large");
  OComplex *data=(OComplex *)malloc(n*n*sizeof(OComplex));
  if(!data) return ofail("out of memory");
  memcpy(data,tensor.as.ctensor.data,n*n*sizeof(OComplex));
  OComplex det={1,0};
  for(size_t i=0;i<n;i++) {
    size_t pivot=i; double max_abs=hypot(data[i*n+i].real,data[i*n+i].imag);
    for(size_t row=i+1;row<n;row++) { double current=hypot(data[row*n+i].real,data[row*n+i].imag); if(current>max_abs) { max_abs=current; pivot=row; } }
    if(max_abs==0) { free(data); return ocomplex(0,0); }
    if(pivot!=i) { for(size_t col=0;col<n;col++) { OComplex temp=data[i*n+col]; data[i*n+col]=data[pivot*n+col]; data[pivot*n+col]=temp; } det.real=-det.real; det.imag=-det.imag; }
    OComplex piv=data[i*n+i]; det=ocmul(det,piv);
    for(size_t row=i+1;row<n;row++) { OComplex factor=ocdiv(data[row*n+i],piv); for(size_t col=i;col<n;col++) data[row*n+col]=ocsub(data[row*n+col],ocmul(factor,data[i*n+col])); }
  }
  free(data);
  return ocomplex(det.real,det.imag);
}
static OValue osolve(OValue matrix, OValue rhs) {
  if(matrix.kind!=O_COMPLEX_TENSOR&&rhs.kind!=O_COMPLEX_TENSOR) return osolve_real(matrix,rhs);
  if((matrix.kind!=O_TENSOR&&matrix.kind!=O_COMPLEX_TENSOR)) return ofail("solve expects a square matrix");
  size_t rank=matrix.kind==O_TENSOR?matrix.as.tensor.rank:matrix.as.ctensor.rank;
  const size_t *shape=matrix.kind==O_TENSOR?matrix.as.tensor.shape:matrix.as.ctensor.shape;
  if(rank!=2||shape[0]!=shape[1]) return ofail("solve expects a square matrix");
  if(rhs.kind!=O_TENSOR&&rhs.kind!=O_COMPLEX_TENSOR) return ofail("solve rhs must be a vector or matrix");
  size_t rhs_rank=rhs.kind==O_TENSOR?rhs.as.tensor.rank:rhs.as.ctensor.rank;
  const size_t *rhs_shape=rhs.kind==O_TENSOR?rhs.as.tensor.shape:rhs.as.ctensor.shape;
  size_t n=shape[0],m;
  if(rhs_rank==1) { if(rhs_shape[0]!=n) return ofail("solve dimension mismatch"); m=1; }
  else if(rhs_rank==2) { if(rhs_shape[0]!=n) return ofail("solve dimension mismatch"); m=rhs_shape[1]; }
  else return ofail("solve rhs must be a vector or matrix");
  if(n==0) return ofail("solve expects a non-empty matrix");
  if(m>SIZE_MAX-n||n>(SIZE_MAX/(n+m))||n*(n+m)>SIZE_MAX/sizeof(OComplex)) return ofail("matrix is too large");
  size_t width=n+m; OComplex *aug=(OComplex *)malloc(n*width*sizeof(OComplex));
  if(!aug) return ofail("out of memory");
  for(size_t row=0;row<n;row++) {
    for(size_t col=0;col<n;col++) aug[row*width+col]=matrix.kind==O_TENSOR?(OComplex){matrix.as.tensor.data[row*n+col],0}:matrix.as.ctensor.data[row*n+col];
    for(size_t col=0;col<m;col++) { size_t index=rhs_rank==1?row:row*m+col; aug[row*width+n+col]=rhs.kind==O_TENSOR?(OComplex){rhs.as.tensor.data[index],0}:rhs.as.ctensor.data[index]; }
  }
  for(size_t i=0;i<n;i++) {
    size_t pivot=i; double max_abs=hypot(aug[i*width+i].real,aug[i*width+i].imag);
    for(size_t row=i+1;row<n;row++) { double current=hypot(aug[row*width+i].real,aug[row*width+i].imag); if(current>max_abs) { max_abs=current; pivot=row; } }
    if(max_abs==0) { free(aug); return ofail("solve matrix is singular"); }
    if(pivot!=i) for(size_t col=0;col<width;col++) { OComplex temporary=aug[i*width+col]; aug[i*width+col]=aug[pivot*width+col]; aug[pivot*width+col]=temporary; }
    OComplex piv=aug[i*width+i];
    for(size_t col=i;col<width;col++) aug[i*width+col]=ocdiv(aug[i*width+col],piv);
    for(size_t row=0;row<n;row++) { if(row==i) continue; OComplex factor=aug[row*width+i]; for(size_t col=i;col<width;col++) aug[row*width+col]=ocsub(aug[row*width+col],ocmul(factor,aug[i*width+col])); }
  }
  if(n&&m>SIZE_MAX/n) { free(aug); return ofail("matrix is too large"); }
  size_t count=n*m; OComplex *data=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL;
  if(count&&!data) { free(aug); return ofail("out of memory"); }
  for(size_t row=0;row<n;row++) if(m) memcpy(data+row*m,aug+row*width+n,m*sizeof(OComplex));
  free(aug);
  if(m==1) { size_t out_shape[1]={n}; return ocomplextensordata(data,count,out_shape,1); }
  size_t out_shape[2]={n,m}; return ocomplextensordata(data,count,out_shape,2);
}
static OValue oinv(OValue matrix) {
  if(matrix.kind!=O_COMPLEX_TENSOR) return oinv_real(matrix);
  if(matrix.as.ctensor.rank!=2||matrix.as.ctensor.shape[0]!=matrix.as.ctensor.shape[1]) return ofail("inv expects a square matrix");
  size_t n=matrix.as.ctensor.shape[0];
  if(n==0) return ofail("inv expects a non-empty matrix");
  if(n>SIZE_MAX/n) return ofail("matrix is too large");
  size_t count=n*n; OComplex *data=(OComplex *)calloc(count,sizeof(OComplex));
  if(!data) return ofail("out of memory");
  for(size_t i=0;i<n;i++) data[i*n+i]=(OComplex){1,0};
  size_t shape[2]={n,n}; OValue rhs=ocomplextensordata(data,count,shape,2); return osolve(matrix,rhs);
}
static OValue ocholesky(OValue matrix) {
  if(matrix.kind!=O_COMPLEX_TENSOR) return ocholesky_real(matrix);
  if(matrix.as.ctensor.rank!=2||matrix.as.ctensor.shape[0]!=matrix.as.ctensor.shape[1]) return ofail("cholesky expects a square matrix");
  size_t n=matrix.as.ctensor.shape[0];
  if(n&&n>SIZE_MAX/n) return ofail("matrix is too large");
  size_t count=n*n;
  for(size_t row=0;row<n;row++) {
    if(fabs(matrix.as.ctensor.data[row*n+row].imag)>1e-12) return ofail("cholesky expects a Hermitian matrix");
    for(size_t col=row+1;col<n;col++) { OComplex a=matrix.as.ctensor.data[row*n+col],b=matrix.as.ctensor.data[col*n+row]; if(hypot(a.real-b.real,a.imag+b.imag)>1e-12) return ofail("cholesky expects a Hermitian matrix"); }
  }
  OComplex *data=count?(OComplex *)calloc(count,sizeof(OComplex)):NULL;
  if(count&&!data) return ofail("out of memory");
  for(size_t row=0;row<n;row++) for(size_t col=0;col<=row;col++) {
    OComplex sum=matrix.as.ctensor.data[row*n+col];
    for(size_t prior=0;prior<col;prior++) { OComplex left=data[row*n+prior],right=data[col*n+prior],product={left.real*right.real+left.imag*right.imag,left.imag*right.real-left.real*right.imag}; sum=ocsub(sum,product); }
    if(row==col) { if(fabs(sum.imag)>1e-10||sum.real<=0) { free(data); return ofail("cholesky expects positive definite matrix"); } data[row*n+col]=(OComplex){sqrt(sum.real),0}; }
    else data[row*n+col]=ocdiv(sum,data[col*n+col]);
  }
  size_t shape[2]={n,n}; return ocomplextensordata(data,count,shape,2);
}
static OValue oqr(OValue matrix) {
  if(matrix.kind!=O_COMPLEX_TENSOR) return oqr_real(matrix);
  if(matrix.as.ctensor.rank!=2) return ofail("qr expects a matrix");
  size_t rows=matrix.as.ctensor.shape[0],cols=matrix.as.ctensor.shape[1];
  if((rows&&cols>SIZE_MAX/rows)||(cols&&cols>SIZE_MAX/cols)) return ofail("matrix is too large");
  size_t qcount=rows*cols,rcount=cols*cols;
  if(qcount>SIZE_MAX/sizeof(OComplex)||rcount>SIZE_MAX/sizeof(OComplex)||rows>SIZE_MAX/sizeof(OComplex)) return ofail("matrix is too large");
  OComplex *q=qcount?(OComplex *)calloc(qcount,sizeof(OComplex)):NULL,*r=rcount?(OComplex *)calloc(rcount,sizeof(OComplex)):NULL,*v=rows?(OComplex *)malloc(rows*sizeof(OComplex)):NULL;
  if((qcount&&!q)||(rcount&&!r)||(rows&&!v)) { free(q); free(r); free(v); return ofail("out of memory"); }
  for(size_t col=0;col<cols;col++) {
    for(size_t row=0;row<rows;row++) v[row]=matrix.as.ctensor.data[row*cols+col];
    for(size_t prior=0;prior<col;prior++) {
      OComplex projection={0,0};
      for(size_t row=0;row<rows;row++) { OComplex basis=q[row*cols+prior],source=matrix.as.ctensor.data[row*cols+col]; projection.real+=basis.real*source.real+basis.imag*source.imag; projection.imag+=basis.real*source.imag-basis.imag*source.real; }
      r[prior*cols+col]=projection;
      for(size_t row=0;row<rows;row++) v[row]=ocsub(v[row],ocmul(projection,q[row*cols+prior]));
    }
    double norm=0; for(size_t row=0;row<rows;row++) norm+=v[row].real*v[row].real+v[row].imag*v[row].imag; norm=sqrt(norm);
    if(norm==0) continue;
    r[col*cols+col]=(OComplex){norm,0};
    for(size_t row=0;row<rows;row++) q[row*cols+col]=(OComplex){v[row].real/norm,v[row].imag/norm};
  }
  free(v); size_t qshape[2]={rows,cols},rshape[2]={cols,cols}; OValue values[2]={ocomplextensordata(q,qcount,qshape,2),ocomplextensordata(r,rcount,rshape,2)}; return olist(values,2);
}
static OValue oeig(OValue tensor) {
  if(tensor.kind!=O_TENSOR||tensor.as.tensor.rank!=2||tensor.as.tensor.shape[0]!=tensor.as.tensor.shape[1]) return ofail("eig expects a square matrix");
  size_t n=tensor.as.tensor.shape[0];
  if(n&&n>SIZE_MAX/n) return ofail("matrix is too large");
  size_t count=n*n;
  double *input=count?(double *)malloc(count*sizeof(double)):NULL;
  OComplex *values=n?(OComplex *)malloc(n*sizeof(OComplex)):NULL,*vectors=count?(OComplex *)malloc(count*sizeof(OComplex)):NULL;
  if((count&&!input)||(n&&!values)||(count&&!vectors)) { free(input); free(values); free(vectors); return ofail("out of memory"); }
  if(count) memcpy(input,tensor.as.tensor.data,count*sizeof(double));
  const OLinalgProviderV1 *provider=osiris_linalg_provider_v1();
  const OLinalgProviderV2 *provider_v2=(const OLinalgProviderV2 *)provider;
  OLinalgStatus status=provider&&provider->abi_version==2&&provider_v2->eig_f64?provider_v2->eig_f64(n,input,values,vectors):O_LINALG_UNAVAILABLE;
  free(input);
  if(status!=O_LINALG_OK) { free(values); free(vectors); return ofail(status==O_LINALG_NOT_CONVERGED?"eig did not converge":"eig numerical provider failed"); }
  size_t value_shape[1]={n},vector_shape[2]={n,n};
  OValue output[2]={ocomplextensordata(values,n,value_shape,1),ocomplextensordata(vectors,count,vector_shape,2)};
  return olist(output,2);
}
`
