package osiris

// cRuntimeNetworkComponent is the integrated POSIX native-C socket adapter for
// std/net. The native emitter appends it alongside the host-resource and
// TLS/HTTP components, and the shared resource dispatcher checks each handle's
// magic before routing reads, writes, or close operations. TLS and HTTP have
// separate provider/bridge components because their resource and map contracts
// are richer than a raw socket.
const cRuntimeNetworkComponent = `
#if !defined(OSIRIS_WINDOWS_COMPONENT)
#define OSIRIS_NATIVE_NETWORK_COMPONENT 1
#if !defined(OSIRIS_FORCE_NONPOSIX) && (defined(__unix__) || defined(__APPLE__))
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/in.h>
#include <poll.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>
#define OSIRIS_NATIVE_POSIX_NET 1
#else
#define OSIRIS_NATIVE_POSIX_NET 0
#endif

#define OSIRIS_NET_MAGIC UINT64_C(0x4f535249534e4554)
#define OSIRIS_NET_MAX_ADDRESS 4096u
#define OSIRIS_NET_MAX_PACKET (1024u * 1024u)
#define OSIRIS_NET_MAX_DNS 65536u

typedef enum { OSIRIS_NET_TCP = 1, OSIRIS_NET_TCP_LISTENER = 2, OSIRIS_NET_UDP = 3 } ONetKind;
typedef struct {
  uint64_t magic;
  int fd;
  ONetKind kind;
  int closed;
} ONetHandle;

static OValue onet_error(const char *operation, const char *resource, const char *detail) {
  OValue error=oerrorvalue("network", detail ? detail : operation);
  if(error.kind==O_ERROR) {
    error.as.error.operation=(OString){operation,strlen(operation)};
    error.as.error.resource=(OString){resource ? resource : "",resource ? strlen(resource) : 0};
    error.as.error.retryable=1;
  }
  olasterror=error; ohaderror=1; oerrormessage=detail ? detail : operation; return onull();
}

static int onet_string(OValue value, const char **data, size_t *length, const char *operation) {
  if(value.kind!=O_STRING || value.as.s.len>OSIRIS_NET_MAX_ADDRESS || memchr(value.as.s.data,0,value.as.s.len)) {
    onet_error(operation,"", "network address must be a bounded NUL-free string"); return 0;
  }
  *data=value.as.s.data; *length=value.as.s.len; return 1;
}

static int onet_i64(OValue value, int64_t *out, const char *operation) {
  double number;
  if(!onum(value,&number) || !isfinite(number) || trunc(number)!=number || number<0 || number>2147483647.0) {
    onet_error(operation,"", "timeout or size must be a non-negative bounded integer"); return 0;
  }
  *out=(int64_t)number; return 1;
}

static ONetHandle *onet_handle(OValue value, ONetKind expected, const char *operation) {
  if(value.kind!=O_RESOURCE || !value.as.resource) { onet_error(operation,"", "expected an open native network resource"); return NULL; }
  ONetHandle *handle=(ONetHandle *)value.as.resource;
  if(handle->magic!=OSIRIS_NET_MAGIC || handle->closed || handle->fd<0 || (expected && handle->kind!=expected)) {
    onet_error(operation,"", "network resource is closed or has the wrong kind"); return NULL;
  }
  return handle;
}

static OValue onet_value(ONetHandle *handle) {
  return (OValue){O_RESOURCE,0,{.ffi={handle}}};
}

static ONetHandle *onet_new(int fd, ONetKind kind, const char *operation, const char *resource) {
  if(fd<0) { onet_error(operation,resource,"invalid network descriptor"); return NULL; }
  ONetHandle *handle=(ONetHandle *)malloc(sizeof(*handle));
  if(!handle) {
#if OSIRIS_NATIVE_POSIX_NET
    close(fd);
#endif
    onet_error(operation,resource,"out of memory"); return NULL;
  }
  handle->magic=OSIRIS_NET_MAGIC; handle->fd=fd; handle->kind=kind; handle->closed=0;
  if(!otrack(handle)) {
#if OSIRIS_NATIVE_POSIX_NET
    close(fd);
#endif
    free(handle); onet_error(operation,resource,"out of memory"); return NULL;
  }
  return handle;
}

static int onet_wait(int fd, short events, int64_t timeout_ms, const char *operation, const char *resource) {
#if OSIRIS_NATIVE_POSIX_NET
  struct pollfd descriptor={fd,events,0};
  int timeout=timeout_ms==0?-1:(int)timeout_ms;
  for(;;) {
    int result=poll(&descriptor,1,timeout);
    if(result>0) {
      // POLLHUP can be reported together with readable response bytes when a
      // peer sends its final data and then closes (the HTTP transport uses
      // Connection: close).  Let the subsequent recv/read observe those bytes;
      // only descriptor errors are terminal before the I/O operation runs.
      if(descriptor.revents&(POLLERR|POLLNVAL)) { onet_error(operation,resource,"network peer closed or descriptor failed"); return 0; }
      if(descriptor.revents&events) return 1;
      if(descriptor.revents&POLLHUP) { onet_error(operation,resource,"network peer closed"); return 0; }
      return 1;
    }
    if(result==0) { onet_error(operation,resource,"network operation timed out"); return 0; }
    if(errno==EINTR) continue;
    onet_error(operation,resource,strerror(errno)); return 0;
  }
#else
  (void)fd; (void)events; (void)timeout_ms; onet_error(operation,resource,"native networking is unavailable on this platform"); return 0;
#endif
}

static int onet_copy_address(const char *data, size_t length, char **out, const char *operation) {
  char *copy=(char *)malloc(length+1);
  if(!copy) { onet_error(operation,data,"out of memory"); return 0; }
  memcpy(copy,data,length); copy[length]=0; *out=copy; return 1;
}

#if OSIRIS_NATIVE_POSIX_NET
static int onet_resolve(const char *address, int type, int flags, struct addrinfo **out) {
  if (!address || !out) return EAI_FAIL;
  char *copy = strdup(address);
  if (!copy) return EAI_MEMORY;
  char *host = copy;
  char *service = NULL;
  if (copy[0] == '[') {
    char *end = strchr(copy, ']');
    if (!end) { free(copy); return EAI_NONAME; }
    *end = 0; host = copy + 1;
    if (end[1] == ':') service = end + 2;
  } else {
    char *first = strchr(copy, ':');
    char *last = strrchr(copy, ':');
    if (first && first == last && last[1] != 0) {
      int numeric = 1; for (char *p = last + 1; *p; p++) if (*p < '0' || *p > '9') { numeric = 0; break; }
      if (numeric) { *last = 0; service = last + 1; }
    }
  }
  struct addrinfo hints; memset(&hints, 0, sizeof(hints));
  hints.ai_socktype = type; hints.ai_family = AF_UNSPEC; hints.ai_flags = flags;
  int result = getaddrinfo(host[0] ? host : NULL, service, &hints, out);
  free(copy); return result;
}
#else
static int onet_resolve(const char *address, int type, int flags, void *out) {
  (void)address; (void)type; (void)flags; (void)out; return -1;
}
#endif

static int onet_connect_fd(const char *address, int64_t timeout_ms, int type, const char *operation) {
#if OSIRIS_NATIVE_POSIX_NET
  struct addrinfo *list=NULL;
  int lookup=onet_resolve(address,type,0,&list);
  if(lookup!=0) { onet_error(operation,address,gai_strerror(lookup)); return -1; }
  int fd=-1;
  for(struct addrinfo *item=list;item;item=item->ai_next) {
    fd=socket(item->ai_family,item->ai_socktype,item->ai_protocol); if(fd<0) continue;
    int flags=-1;
    if(type==SOCK_STREAM && timeout_ms>0) {
      flags=fcntl(fd,F_GETFL,0); if(flags<0 || fcntl(fd,F_SETFL,flags|O_NONBLOCK)<0) { close(fd); fd=-1; continue; }
    }
    if(connect(fd,item->ai_addr,item->ai_addrlen)==0) break;
    if(type==SOCK_STREAM && timeout_ms>0 && (errno==EINPROGRESS || errno==EWOULDBLOCK)) {
      struct pollfd descriptor={fd,POLLOUT,0}; int waitResult=poll(&descriptor,1,(int)timeout_ms);
      if(waitResult>0 && !(descriptor.revents&(POLLERR|POLLHUP|POLLNVAL))) {
        int error=0; socklen_t errorLength=sizeof(error); getsockopt(fd,SOL_SOCKET,SO_ERROR,&error,&errorLength);
        if(error==0) { fcntl(fd,F_SETFL,flags); break; }
      }
    }
    close(fd); fd=-1;
  }
  freeaddrinfo(list);
  if(fd<0) { onet_error(operation,address,"unable to connect to network address"); return -1; }
  return fd;
#else
  (void)address; (void)timeout_ms; (void)type; onet_error(operation,address,"native networking is unavailable on this platform"); return -1;
#endif
}

static OValue onet_tcp_dial(OValue addressValue, OValue timeoutValue) {
#if OSIRIS_NATIVE_POSIX_NET
  const char *address; size_t length; if(!onet_string(addressValue,&address,&length,"tcp_dial")) return onull();
  int64_t timeout=0; if(timeoutValue.kind!=O_NULL&&!onet_i64(timeoutValue,&timeout,"tcp_dial")) return onull();
  char *copy=NULL; if(!onet_copy_address(address,length,&copy,"tcp_dial")) return onull();
  int fd=onet_connect_fd(copy,timeout,SOCK_STREAM,"tcp_dial");
  ONetHandle *handle=fd>=0?onet_new(fd,OSIRIS_NET_TCP,"tcp_dial",copy):NULL; free(copy);
  return handle?onet_value(handle):onull();
#else
  (void)addressValue; (void)timeoutValue; return onet_error("tcp_dial","","native networking is unavailable on this platform");
#endif
}

static OValue onet_tcp_listen(OValue addressValue) {
#if OSIRIS_NATIVE_POSIX_NET
  const char *address; size_t length; if(!onet_string(addressValue,&address,&length,"tcp_listen")) return onull();
  char *copy=NULL; if(!onet_copy_address(address,length,&copy,"tcp_listen")) return onull();
  struct addrinfo *list=NULL; int lookup=onet_resolve(copy,SOCK_STREAM,AI_PASSIVE,&list); if(lookup!=0) { free(copy); return onet_error("tcp_listen",address,gai_strerror(lookup)); }
  int fd=-1;
  for(struct addrinfo *item=list;item;item=item->ai_next) { fd=socket(item->ai_family,item->ai_socktype,item->ai_protocol); if(fd<0) continue; int yes=1; setsockopt(fd,SOL_SOCKET,SO_REUSEADDR,&yes,sizeof(yes)); if(bind(fd,item->ai_addr,item->ai_addrlen)==0 && listen(fd,128)==0) break; close(fd); fd=-1; }
  freeaddrinfo(list); ONetHandle *handle=fd>=0?onet_new(fd,OSIRIS_NET_TCP_LISTENER,"tcp_listen",copy):NULL; if(!handle&&fd>=0) close(fd); const char *detail=fd<0?strerror(errno):"unable to bind or listen"; free(copy); return handle?onet_value(handle):onet_error("tcp_listen",address,detail);
#else
  (void)addressValue; return onet_error("tcp_listen","","native networking is unavailable on this platform");
#endif
}

static OValue onet_tcp_accept(OValue listenerValue, OValue timeoutValue) {
#if OSIRIS_NATIVE_POSIX_NET
  ONetHandle *listener=onet_handle(listenerValue,OSIRIS_NET_TCP_LISTENER,"tcp_accept"); if(!listener) return onull();
  int64_t timeout=0; if(timeoutValue.kind!=O_NULL&&!onet_i64(timeoutValue,&timeout,"tcp_accept")) return onull();
  if(!onet_wait(listener->fd,POLLIN,timeout,"tcp_accept","listener")) return onull();
  int fd=accept(listener->fd,NULL,NULL); if(fd<0) return onet_error("tcp_accept","listener",strerror(errno));
  ONetHandle *handle=onet_new(fd,OSIRIS_NET_TCP,"tcp_accept","accepted peer"); return handle?onet_value(handle):onull();
#else
  (void)listenerValue; (void)timeoutValue; return onet_error("tcp_accept","","native networking is unavailable on this platform");
#endif
}

static OValue onet_read(OValue socketValue, OValue sizeValue, OValue timeoutValue) {
#if OSIRIS_NATIVE_POSIX_NET
  ONetHandle *socketHandle=onet_handle(socketValue,OSIRIS_NET_TCP,"readchunk"); if(!socketHandle) return onull();
  int64_t size=0,timeout=0; if(!onet_i64(sizeValue,&size,"readchunk")||size<0||size>OSIRIS_NET_MAX_PACKET) return onull(); if(timeoutValue.kind!=O_NULL&&!onet_i64(timeoutValue,&timeout,"readchunk")) return onull();
  if(!onet_wait(socketHandle->fd,POLLIN,timeout,"readchunk","tcp")) return onull();
  unsigned char *data=size?(unsigned char *)malloc((size_t)size):NULL; if(size&&!data) return onet_error("readchunk","tcp","out of memory"); ssize_t count=recv(socketHandle->fd,data,(size_t)size,0); if(count<0) { free(data); return onet_error("readchunk","tcp",strerror(errno)); }
  OValue *items=count?(OValue *)malloc((size_t)count*sizeof(OValue)):NULL; if(count&&!items) { free(data); return onet_error("readchunk","tcp","out of memory"); } for(ssize_t i=0;i<count;i++) items[i]=ouintbits(data[i],8); OValue result=olist(items,(size_t)count); free(items); free(data); return result;
#else
  (void)socketValue; (void)sizeValue; (void)timeoutValue; return onet_error("readchunk","","native networking is unavailable on this platform");
#endif
}

static OValue onet_write(OValue socketValue, OValue dataValue, OValue timeoutValue) {
#if OSIRIS_NATIVE_POSIX_NET
  ONetHandle *socketHandle=onet_handle(socketValue,OSIRIS_NET_TCP,"writechunk"); if(!socketHandle) return onull();
  const unsigned char *data=NULL; size_t length=0; unsigned char *owned=NULL; if(dataValue.kind==O_STRING) { data=(const unsigned char *)dataValue.as.s.data; length=dataValue.as.s.len; } else if(dataValue.kind==O_LIST) { if(dataValue.as.list.len>OSIRIS_NET_MAX_PACKET) return onet_error("writechunk","tcp","write exceeds packet limit"); owned=(unsigned char *)malloc(dataValue.as.list.len); if(dataValue.as.list.len&&!owned) return onet_error("writechunk","tcp","out of memory"); for(size_t i=0;i<dataValue.as.list.len;i++) { if(dataValue.as.list.items[i].kind!=O_UINT||dataValue.as.list.items[i].as.u>255) { free(owned); return onet_error("writechunk","tcp","write data must be bytes"); } owned[i]=(unsigned char)dataValue.as.list.items[i].as.u; } data=owned; length=dataValue.as.list.len; } else return onet_error("writechunk","tcp","write data must be a string or byte list");
  int64_t timeout=0; if(timeoutValue.kind!=O_NULL&&!onet_i64(timeoutValue,&timeout,"writechunk")) { free(owned); return onull(); }
  size_t sent=0; while(sent<length) { if(!onet_wait(socketHandle->fd,POLLOUT,timeout,"writechunk","tcp")) { free(owned); return onull(); } ssize_t count=send(socketHandle->fd,data+sent,length-sent,0); if(count<0) { if(errno==EINTR) continue; free(owned); return onet_error("writechunk","tcp",strerror(errno)); } if(count==0) break; sent+=(size_t)count; }
  free(owned); return ouintbits(sent,64);
#else
  (void)socketValue; (void)dataValue; (void)timeoutValue; return onet_error("writechunk","","native networking is unavailable on this platform");
#endif
}

static OValue onet_udp_bind(OValue addressValue) {
#if OSIRIS_NATIVE_POSIX_NET
  const char *address; size_t length; if(!onet_string(addressValue,&address,&length,"udp_bind")) return onull(); char *copy=NULL; if(!onet_copy_address(address,length,&copy,"udp_bind")) return onull();
  struct addrinfo *list=NULL; int lookup=onet_resolve(copy,SOCK_DGRAM,AI_PASSIVE,&list); if(lookup!=0) { free(copy); return onet_error("udp_bind",address,gai_strerror(lookup)); }
  int fd=-1; for(struct addrinfo *item=list;item;item=item->ai_next) { fd=socket(item->ai_family,item->ai_socktype,item->ai_protocol); if(fd<0) continue; int yes=1; setsockopt(fd,SOL_SOCKET,SO_REUSEADDR,&yes,sizeof(yes)); if(bind(fd,item->ai_addr,item->ai_addrlen)==0) break; close(fd); fd=-1; } const char *detail=fd<0?strerror(errno):"unable to bind UDP socket"; freeaddrinfo(list); ONetHandle *handle=fd>=0?onet_new(fd,OSIRIS_NET_UDP,"udp_bind",copy):NULL; if(!handle&&fd>=0) close(fd); free(copy); return handle?onet_value(handle):onet_error("udp_bind",address,detail);
#else
  (void)addressValue; return onet_error("udp_bind","","native networking is unavailable on this platform");
#endif
}

static OValue onet_udp_connect(OValue addressValue) {
#if OSIRIS_NATIVE_POSIX_NET
  const char *address; size_t length; if(!onet_string(addressValue,&address,&length,"udp_connect")) return onull(); char *copy=NULL; if(!onet_copy_address(address,length,&copy,"udp_connect")) return onull(); int fd=onet_connect_fd(copy,0,SOCK_DGRAM,"udp_connect"); ONetHandle *handle=fd>=0?onet_new(fd,OSIRIS_NET_UDP,"udp_connect",copy):NULL; free(copy); return handle?onet_value(handle):onull();
#else
  (void)addressValue; return onet_error("udp_connect","","native networking is unavailable on this platform");
#endif
}

static OValue onet_udp_send(OValue socketValue, OValue dataValue, OValue peerValue) {
#if OSIRIS_NATIVE_POSIX_NET
  ONetHandle *socketHandle=onet_handle(socketValue,OSIRIS_NET_UDP,"udp_send"); if(!socketHandle) return onull(); const unsigned char *data=NULL; size_t length=0; unsigned char *owned=NULL; if(dataValue.kind==O_STRING) { data=(const unsigned char *)dataValue.as.s.data; length=dataValue.as.s.len; } else if(dataValue.kind==O_LIST) { length=dataValue.as.list.len; owned=(unsigned char *)malloc(length); if(length&&!owned) return onet_error("udp_send","udp","out of memory"); for(size_t i=0;i<length;i++) { if(dataValue.as.list.items[i].kind!=O_UINT||dataValue.as.list.items[i].as.u>255) { free(owned); return onet_error("udp_send","udp","packet data must be bytes"); } owned[i]=(unsigned char)dataValue.as.list.items[i].as.u; } data=owned; } else return onet_error("udp_send","udp","packet data must be a string or byte list"); if(length>OSIRIS_NET_MAX_PACKET) { free(owned); return onet_error("udp_send","udp","packet exceeds the 1048576-byte limit"); }
  ssize_t sent=-1; if(peerValue.kind==O_NULL) sent=send(socketHandle->fd,data,length,0); else { const char *peer; size_t peerLength; if(!onet_string(peerValue,&peer,&peerLength,"udp_send")) { free(owned); return onull(); } char *copy=NULL; if(!onet_copy_address(peer,peerLength,&copy,"udp_send")) { free(owned); return onull(); } struct addrinfo *list=NULL; int lookup=onet_resolve(copy,SOCK_DGRAM,0,&list); if(lookup==0&&list) sent=sendto(socketHandle->fd,data,length,0,list->ai_addr,list->ai_addrlen); if(lookup!=0) onet_error("udp_send",copy,gai_strerror(lookup)); freeaddrinfo(list); free(copy); } free(owned); if(sent<0) return onet_error("udp_send","udp",strerror(errno)); return ouintbits((uint64_t)sent,64);
#else
  (void)socketValue; (void)dataValue; (void)peerValue; return onet_error("udp_send","","native networking is unavailable on this platform");
#endif
}

static OValue onet_udp_receive(OValue socketValue, OValue sizeValue, OValue timeoutValue) {
#if OSIRIS_NATIVE_POSIX_NET
  ONetHandle *socketHandle=onet_handle(socketValue,OSIRIS_NET_UDP,"udp_receive"); if(!socketHandle) return onull(); int64_t size=OSIRIS_NET_MAX_PACKET,timeout=0; if(sizeValue.kind!=O_NULL&&!onet_i64(sizeValue,&size,"udp_receive")) return onull(); if(size<0||size>OSIRIS_NET_MAX_PACKET) return onet_error("udp_receive","udp","receive size exceeds the 1048576-byte limit"); if(timeoutValue.kind!=O_NULL&&!onet_i64(timeoutValue,&timeout,"udp_receive")) return onull(); if(!onet_wait(socketHandle->fd,POLLIN,timeout,"udp_receive","udp")) return onull(); unsigned char *data=size?(unsigned char *)malloc((size_t)size):NULL; if(size&&!data) return onet_error("udp_receive","udp","out of memory"); struct sockaddr_storage peer; socklen_t peerLength=sizeof(peer); ssize_t count=recvfrom(socketHandle->fd,data,(size_t)size,0,(struct sockaddr *)&peer,&peerLength); if(count<0) { free(data); return onet_error("udp_receive","udp",strerror(errno)); } char peerText[NI_MAXHOST+NI_MAXSERV+4]; int rendered=getnameinfo((struct sockaddr *)&peer,peerLength,peerText,sizeof(peerText),peerText+NI_MAXHOST+2,NI_MAXSERV,NI_NUMERICHOST|NI_NUMERICSERV); if(rendered!=0) snprintf(peerText,sizeof(peerText),"unknown"); OValue *items=count?(OValue *)malloc((size_t)count*sizeof(OValue)):NULL; if(count&&!items) { free(data); return onet_error("udp_receive","udp","out of memory"); } for(ssize_t i=0;i<count;i++) items[i]=ouintbits(data[i],8); OValue bytes=olist(items,(size_t)count); free(items); free(data); OValue tuple[3]={bytes,ostring(peerText),obool(0)}; return olist(tuple,3);
#else
  (void)socketValue; (void)sizeValue; (void)timeoutValue; return onet_error("udp_receive","","native networking is unavailable on this platform");
#endif
}

static OValue onet_dns_lookup(OValue hostValue, OValue policyValue) {
#if OSIRIS_NATIVE_POSIX_NET
  const char *host; size_t length; if(!onet_string(hostValue,&host,&length,"dns_lookup")) return onull(); if(policyValue.kind!=O_NULL && policyValue.kind!=O_STRING) return onet_error("dns_lookup",host,"resolver policy must be a string"); if(policyValue.kind==O_STRING && strcmp(policyValue.as.s.data,"system")!=0 && strcmp(policyValue.as.s.data,"go")!=0) return onet_error("dns_lookup",host,"resolver policy must be system or go"); char *copy=NULL; if(!onet_copy_address(host,length,&copy,"dns_lookup")) return onull(); struct addrinfo hints; memset(&hints,0,sizeof(hints)); hints.ai_family=AF_UNSPEC; hints.ai_socktype=SOCK_STREAM; struct addrinfo *list=NULL; int lookup=getaddrinfo(copy,NULL,&hints,&list); if(lookup!=0) { free(copy); return onet_error("dns_lookup",host,gai_strerror(lookup)); }
  char **values=(char **)calloc(OSIRIS_NET_MAX_DNS,sizeof(char *)); size_t count=0; for(struct addrinfo *item=list;item&&count<OSIRIS_NET_MAX_DNS;item=item->ai_next) { char text[NI_MAXHOST]; if(getnameinfo(item->ai_addr,item->ai_addrlen,text,sizeof(text),NULL,0,NI_NUMERICHOST)!=0) continue; int duplicate=0; for(size_t i=0;i<count;i++) if(!strcmp(values[i],text)) { duplicate=1; break; } if(!duplicate) { values[count]=strdup(text); if(!values[count]) { for(size_t i=0;i<count;i++) free(values[i]); free(values); freeaddrinfo(list); free(copy); return onet_error("dns_lookup",host,"out of memory"); } count++; } } freeaddrinfo(list); free(copy); for(size_t i=1;i<count;i++) { char *value=values[i]; size_t j=i; while(j>0&&strcmp(values[j-1],value)>0) { values[j]=values[j-1]; j--; } values[j]=value; } OValue *items=count?(OValue *)malloc(count*sizeof(OValue)):NULL; if(count&&!items) { for(size_t i=0;i<count;i++) free(values[i]); free(values); return onet_error("dns_lookup",host,"out of memory"); } for(size_t i=0;i<count;i++) { items[i]=oownedstring(values[i],strlen(values[i])); free(values[i]); } free(values); return olist(items,count);
#else
  (void)hostValue; (void)policyValue; return onet_error("dns_lookup","","native networking is unavailable on this platform");
#endif
}

static OValue onet_close(OValue value) {
#if OSIRIS_NATIVE_POSIX_NET
  if(value.kind!=O_RESOURCE || !value.as.resource) return onet_error("close","","expected an open native network resource"); ONetHandle *handle=(ONetHandle *)value.as.resource; if(handle->magic!=OSIRIS_NET_MAGIC) return onet_error("close","","resource is not a native network resource"); if(handle->closed) return obool(0); int result=close(handle->fd); handle->fd=-1; handle->closed=1; if(result!=0) return onet_error("close","network",strerror(errno)); return obool(1);
#else
  (void)value; return onet_error("close","","native networking is unavailable on this platform");
#endif
}

static OValue onet_tls_dial(OValue address, OValue serverName, OValue roots, OValue timeout) { (void)address; (void)serverName; (void)roots; (void)timeout; return onet_error("tls_dial","","TLS requires an explicitly linked native TLS provider"); }
static OValue onet_tls_listen(OValue address, OValue certificate, OValue key) { (void)address; (void)certificate; (void)key; return onet_error("tls_listen","","TLS requires an explicitly linked native TLS provider"); }
static OValue onet_http_request(const OValue *args, size_t argc) { (void)args; (void)argc; return onet_error("http_request","","native HTTP requires the native map and cancellation ABIs"); }
static OValue onet_http_serve(const OValue *args, size_t argc) { (void)args; (void)argc; return onet_error("http_serve","","native HTTP requires the native map, closure, and cancellation ABIs"); }
#endif
`

// nativeNetworkRuntimeMembers is the public native-C networking admission set.
func nativeNetworkRuntimeMembers() map[string][]string {
	return map[string][]string{
		"std/net": {
			"tcp_listen", "tcp_accept", "tcp_dial", "udp_bind", "udp_connect",
			"udp_send", "udp_receive", "dns_lookup", "tls_dial", "tls_dial_config",
			"tls_listen", "http_get", "http_request", "http_request_stream", "http_serve",
		},
	}
}

func nativeNetworkRuntimeCapabilities() map[string]string {
	return map[string]string{
		"tcp_listen":          "net.tcp_listen",
		"tcp_accept":          "net.tcp_accept",
		"tcp_dial":            "net.tcp_dial",
		"readchunk":           "net.tcp_read",
		"writechunk":          "net.tcp_write",
		"close":               "net.close",
		"udp_bind":            "net.udp_bind",
		"udp_connect":         "net.udp_connect",
		"udp_send":            "net.udp_send",
		"udp_receive":         "net.udp_receive",
		"dns_lookup":          "net.dns_lookup",
		"tls_dial":            "net.tls_dial",
		"tls_dial_config":     "net.tls_dial_config",
		"tls_listen":          "net.tls_listen",
		"http_get":            "net.http_client",
		"http_request":        "net.http_request",
		"http_request_stream": "net.http_stream",
		"http_serve":          "net.http_serve",
	}
}
