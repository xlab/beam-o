# proto.capnp
@0xcda9b570d42545f7;
$import "/go.capnp".package("proto");
struct FileHeader @0xaacf352394949638 {  # 24 bytes, 1 ptrs
  name @0 :Text;  # ptr[0]
  isDir @1 :Bool;  # bits[0, 1)
  mode @2 :UInt32;  # bits[32, 64)
  modTime @3 :Int64;  # bits[64, 128)
  size @4 :Int64;  # bits[128, 192)
}
