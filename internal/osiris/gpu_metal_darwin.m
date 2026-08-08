#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

typedef struct {
    uint32_t a_count;
    uint32_t b_count;
    uint32_t out_count;
} OsirisMetalParams;

static void osiris_metal_error(char *out, size_t cap, NSString *message) {
    if (out == NULL || cap == 0) return;
    const char *text = message.UTF8String ?: "Metal operation failed";
    snprintf(out, cap, "%s", text);
}

static NSString *osiris_metal_expression(int op) {
    switch (op) {
        case 1: return @"x + y";
        case 2: return @"x - y";
        case 3: return @"x * y";
        case 4: return @"x / y";
        case 5: return @"pow(x, y)";
        case 6: return @"hypot(x, y)";
        case 7: return @"fabs(x)";
        case 8: return @"sqrt(x)";
        case 9: return @"exp(x)";
        case 10: return @"log(x)";
        case 11: return @"log10(x)";
        case 12: return @"log2(x)";
        case 13: return @"sin(x)";
        case 14: return @"cos(x)";
        case 15: return @"tan(x)";
        case 16: return @"asin(x)";
        case 17: return @"acos(x)";
        case 18: return @"atan(x)";
        case 19: return @"floor(x)";
        case 20: return @"ceil(x)";
        case 21: return @"rint(x)";
        default: return nil;
    }
}

uintptr_t osiris_metal_create_device(char *name, size_t name_cap) {
    id<MTLDevice> device = MTLCreateSystemDefaultDevice();
    if (device == nil) {
        NSArray<id<MTLDevice>> *devices = MTLCopyAllDevices();
        device = devices.firstObject;
    }
    if (device == nil) return 0;
    if (name != NULL && name_cap > 0) snprintf(name, name_cap, "%s", device.name.UTF8String ?: "Metal device");
    return (uintptr_t)(__bridge_retained void *)device;
}

void osiris_metal_release_device(uintptr_t handle) {
    if (handle != 0) CFRelease((CFTypeRef)(void *)handle);
}

uintptr_t osiris_metal_allocate(uintptr_t device_handle, const float *data, size_t count, char *err, size_t err_cap) {
    id<MTLDevice> device = (__bridge id<MTLDevice>)(void *)device_handle;
    if (device == nil || data == NULL || count == 0) {
        osiris_metal_error(err, err_cap, @"invalid Metal allocation arguments");
        return 0;
    }
    id<MTLBuffer> buffer = [device newBufferWithBytes:data length:count * sizeof(float) options:MTLResourceStorageModeShared];
    if (buffer == nil) {
        osiris_metal_error(err, err_cap, @"Metal buffer allocation returned nil");
        return 0;
    }
    return (uintptr_t)(__bridge_retained void *)buffer;
}

uintptr_t osiris_metal_launch(uintptr_t device_handle, int op, uintptr_t a_handle, size_t a_count, uintptr_t b_handle, size_t b_count, size_t out_count, uintptr_t *out_buffer, char *err, size_t err_cap) {
    id<MTLDevice> device = (__bridge id<MTLDevice>)(void *)device_handle;
    id<MTLBuffer> a = (__bridge id<MTLBuffer>)(void *)a_handle;
    id<MTLBuffer> b = b_handle == 0 ? nil : (__bridge id<MTLBuffer>)(void *)b_handle;
    if (device == nil || a == nil || out_buffer == NULL || a_count == 0 || out_count == 0) {
        osiris_metal_error(err, err_cap, @"invalid Metal launch arguments");
        return 0;
    }
    NSString *expression = osiris_metal_expression(op);
    if (expression == nil) {
        osiris_metal_error(err, err_cap, @"unsupported Metal operation");
        return 0;
    }
    NSString *source = [NSString stringWithFormat:
        @"#include <metal_stdlib>\nusing namespace metal;\n"
         "struct Params { uint a_count; uint b_count; uint out_count; };\n"
         "kernel void main0(device const float *a [[buffer(0)]], device const float *b [[buffer(1)]], device float *out [[buffer(2)]], constant Params& p [[buffer(3)]], uint id [[thread_position_in_grid]]) { if (id >= p.out_count) return; float x = a[id %% p.a_count]; float y = p.b_count == 0 ? 0.0f : b[id %% p.b_count]; out[id] = %@; }\n", expression];
    NSError *error = nil;
    id<MTLLibrary> library = [device newLibraryWithSource:source options:nil error:&error];
    if (library == nil) {
        osiris_metal_error(err, err_cap, error.localizedDescription);
        return 0;
    }
    id<MTLFunction> function = [library newFunctionWithName:@"main0"];
    id<MTLComputePipelineState> pipeline = [device newComputePipelineStateWithFunction:function error:&error];
    if (pipeline == nil) {
        osiris_metal_error(err, err_cap, error.localizedDescription);
        return 0;
    }
    id<MTLBuffer> output = [device newBufferWithLength:out_count * sizeof(float) options:MTLResourceStorageModeShared];
    id<MTLCommandQueue> queue = [device newCommandQueue];
    id<MTLCommandBuffer> command = [queue commandBuffer];
    id<MTLComputeCommandEncoder> encoder = [command computeCommandEncoder];
    if (output == nil || queue == nil || command == nil || encoder == nil) {
        osiris_metal_error(err, err_cap, @"Metal command construction failed");
        return 0;
    }
    [encoder setComputePipelineState:pipeline];
    [encoder setBuffer:a offset:0 atIndex:0];
    if (b != nil) [encoder setBuffer:b offset:0 atIndex:1];
    [encoder setBuffer:output offset:0 atIndex:2];
    OsirisMetalParams params = {(uint32_t)a_count, (uint32_t)b_count, (uint32_t)out_count};
    [encoder setBytes:&params length:sizeof(params) atIndex:3];
    NSUInteger width = pipeline.maxTotalThreadsPerThreadgroup;
    if (width == 0) width = 1;
    if (width > out_count) width = out_count;
    [encoder dispatchThreads:MTLSizeMake(out_count, 1, 1) threadsPerThreadgroup:MTLSizeMake(width, 1, 1)];
    [encoder endEncoding];
    [command commit];
    *out_buffer = (uintptr_t)(__bridge_retained void *)output;
    return (uintptr_t)(__bridge_retained void *)command;
}

int osiris_metal_wait(uintptr_t event_handle, char *err, size_t err_cap) {
    id<MTLCommandBuffer> command = (__bridge id<MTLCommandBuffer>)(void *)event_handle;
    if (command == nil) {
        osiris_metal_error(err, err_cap, @"invalid Metal event");
        return 1;
    }
    [command waitUntilCompleted];
    if (command.status == MTLCommandBufferStatusError) {
        osiris_metal_error(err, err_cap, command.error.localizedDescription);
        return 1;
    }
    return 0;
}

int osiris_metal_read(uintptr_t buffer_handle, float *data, size_t count, char *err, size_t err_cap) {
    id<MTLBuffer> buffer = (__bridge id<MTLBuffer>)(void *)buffer_handle;
    if (buffer == nil || data == NULL || count == 0) {
        osiris_metal_error(err, err_cap, @"invalid Metal readback arguments");
        return 1;
    }
    memcpy(data, buffer.contents, count * sizeof(float));
    return 0;
}

void osiris_metal_release_buffer(uintptr_t handle) {
    if (handle != 0) CFRelease((CFTypeRef)(void *)handle);
}

void osiris_metal_release_event(uintptr_t handle) {
    if (handle != 0) CFRelease((CFTypeRef)(void *)handle);
}

int osiris_metal_supports(int op) {
    return osiris_metal_expression(op) != nil;
}
