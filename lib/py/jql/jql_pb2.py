# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: jql/jql.proto
# Protobuf Python Version: 4.25.0
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\rjql/jql.proto\x12\x03jql\"\x1b\n\nEqualMatch\x12\r\n\x05value\x18\x01 \x01(\t\"\x1e\n\rLessThanMatch\x12\r\n\x05value\x18\x01 \x01(\t\"!\n\x10GreaterThanMatch\x12\r\n\x05value\x18\x01 \x01(\t\"\x19\n\x07InMatch\x12\x0e\n\x06values\x18\x01 \x03(\t\"-\n\rContainsMatch\x12\r\n\x05\x65xact\x18\x01 \x01(\x08\x12\r\n\x05value\x18\x02 \x01(\t\"\x8f\x02\n\x06\x46ilter\x12\x0f\n\x07negated\x18\x01 \x01(\x08\x12\x0e\n\x06\x63olumn\x18\x02 \x01(\t\x12&\n\x0b\x65qual_match\x18\x03 \x01(\x0b\x32\x0f.jql.EqualMatchH\x00\x12-\n\x0fless_than_match\x18\x04 \x01(\x0b\x32\x12.jql.LessThanMatchH\x00\x12\x34\n\x13greather_than_match\x18\x05 \x01(\x0b\x32\x15.jql.GreaterThanMatchH\x00\x12 \n\x08in_match\x18\x06 \x01(\x0b\x32\x0c.jql.InMatchH\x00\x12,\n\x0e\x63ontains_match\x18\x07 \x01(\x0b\x32\x12.jql.ContainsMatchH\x00\x42\x07\n\x05match\"*\n\tCondition\x12\x1d\n\x08requires\x18\x01 \x03(\x0b\x32\x0b.jql.Filter\"\x82\x01\n\x0fListRowsRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\"\n\nconditions\x18\x02 \x03(\x0b\x32\x0e.jql.Condition\x12\x10\n\x08order_by\x18\x03 \x01(\t\x12\x0b\n\x03\x64\x65\x63\x18\x04 \x01(\x08\x12\x0e\n\x06offset\x18\x05 \x01(\r\x12\r\n\x05limit\x18\x06 \x01(\r\"H\n\x06\x43olumn\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x1c\n\x04type\x18\x02 \x01(\x0e\x32\x0e.jql.EntryType\x12\x12\n\nmax_length\x18\x03 \x01(\x05\"\x1a\n\x05\x45ntry\x12\x11\n\tformatted\x18\x01 \x01(\t\"\"\n\x03Row\x12\x1b\n\x07\x65ntries\x18\x01 \x03(\x0b\x32\n.jql.Entry\"W\n\x10ListRowsResponse\x12\x1c\n\x07\x63olumns\x18\x01 \x03(\x0b\x32\x0b.jql.Column\x12\x16\n\x04rows\x18\x02 \x03(\x0b\x32\x08.jql.Row\x12\r\n\x05total\x18\x03 \x01(\r\"*\n\rGetRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\"E\n\x0eGetRowResponse\x12\x1c\n\x07\x63olumns\x18\x01 \x03(\x0b\x32\x0b.jql.Column\x12\x15\n\x03row\x18\x02 \x01(\x0b\x32\x08.jql.Row\"\xb7\x01\n\x0fWriteRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\x12\x30\n\x06\x66ields\x18\x03 \x03(\x0b\x32 .jql.WriteRowRequest.FieldsEntry\x12\x13\n\x0bupdate_only\x18\x04 \x01(\x08\x12\x13\n\x0binsert_only\x18\x05 \x01(\x08\x1a-\n\x0b\x46ieldsEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\t:\x02\x38\x01\"\x12\n\x10WriteRowResponse\"-\n\x10\x44\x65leteRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\"\x13\n\x11\x44\x65leteRowResponse\"\x10\n\x0ePersistRequest\"\x11\n\x0fPersistResponse*o\n\tEntryType\x12\n\n\x06STRING\x10\x00\x12\x07\n\x03INT\x10\x01\x12\x08\n\x04\x44\x41TE\x10\x02\x12\x08\n\x04\x45NUM\x10\x03\x12\x06\n\x02ID\x10\x04\x12\x08\n\x04TIME\x10\x05\x12\x0c\n\x08MONEYAMT\x10\x06\x12\x0b\n\x07\x46OREIGN\x10\x07\x12\x0c\n\x08\x46OREIGNS\x10\x08\x32\x9c\x02\n\x03JQL\x12\x37\n\x08ListRows\x12\x14.jql.ListRowsRequest\x1a\x15.jql.ListRowsResponse\x12\x31\n\x06GetRow\x12\x12.jql.GetRowRequest\x1a\x13.jql.GetRowResponse\x12\x37\n\x08WriteRow\x12\x14.jql.WriteRowRequest\x1a\x15.jql.WriteRowResponse\x12:\n\tDeleteRow\x12\x15.jql.DeleteRowRequest\x1a\x16.jql.DeleteRowResponse\x12\x34\n\x07Persist\x12\x13.jql.PersistRequest\x1a\x14.jql.PersistResponseB\x0bZ\tjql/jqlpbb\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'jql.jql_pb2', _globals)
if _descriptor._USE_C_DESCRIPTORS == False:
  _globals['DESCRIPTOR']._options = None
  _globals['DESCRIPTOR']._serialized_options = b'Z\tjql/jqlpb'
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._options = None
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_options = b'8\001'
  _globals['_ENTRYTYPE']._serialized_start=1296
  _globals['_ENTRYTYPE']._serialized_end=1407
  _globals['_EQUALMATCH']._serialized_start=22
  _globals['_EQUALMATCH']._serialized_end=49
  _globals['_LESSTHANMATCH']._serialized_start=51
  _globals['_LESSTHANMATCH']._serialized_end=81
  _globals['_GREATERTHANMATCH']._serialized_start=83
  _globals['_GREATERTHANMATCH']._serialized_end=116
  _globals['_INMATCH']._serialized_start=118
  _globals['_INMATCH']._serialized_end=143
  _globals['_CONTAINSMATCH']._serialized_start=145
  _globals['_CONTAINSMATCH']._serialized_end=190
  _globals['_FILTER']._serialized_start=193
  _globals['_FILTER']._serialized_end=464
  _globals['_CONDITION']._serialized_start=466
  _globals['_CONDITION']._serialized_end=508
  _globals['_LISTROWSREQUEST']._serialized_start=511
  _globals['_LISTROWSREQUEST']._serialized_end=641
  _globals['_COLUMN']._serialized_start=643
  _globals['_COLUMN']._serialized_end=715
  _globals['_ENTRY']._serialized_start=717
  _globals['_ENTRY']._serialized_end=743
  _globals['_ROW']._serialized_start=745
  _globals['_ROW']._serialized_end=779
  _globals['_LISTROWSRESPONSE']._serialized_start=781
  _globals['_LISTROWSRESPONSE']._serialized_end=868
  _globals['_GETROWREQUEST']._serialized_start=870
  _globals['_GETROWREQUEST']._serialized_end=912
  _globals['_GETROWRESPONSE']._serialized_start=914
  _globals['_GETROWRESPONSE']._serialized_end=983
  _globals['_WRITEROWREQUEST']._serialized_start=986
  _globals['_WRITEROWREQUEST']._serialized_end=1169
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_start=1124
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_end=1169
  _globals['_WRITEROWRESPONSE']._serialized_start=1171
  _globals['_WRITEROWRESPONSE']._serialized_end=1189
  _globals['_DELETEROWREQUEST']._serialized_start=1191
  _globals['_DELETEROWREQUEST']._serialized_end=1236
  _globals['_DELETEROWRESPONSE']._serialized_start=1238
  _globals['_DELETEROWRESPONSE']._serialized_end=1257
  _globals['_PERSISTREQUEST']._serialized_start=1259
  _globals['_PERSISTREQUEST']._serialized_end=1275
  _globals['_PERSISTRESPONSE']._serialized_start=1277
  _globals['_PERSISTRESPONSE']._serialized_end=1294
  _globals['_JQL']._serialized_start=1410
  _globals['_JQL']._serialized_end=1694
# @@protoc_insertion_point(module_scope)
