# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# NO CHECKED-IN PROTOBUF GENCODE
# source: jql/jql.proto
# Protobuf Python Version: 5.29.0
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import runtime_version as _runtime_version
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
_runtime_version.ValidateProtobufRuntimeVersion(
    _runtime_version.Domain.PUBLIC,
    5,
    29,
    0,
    '',
    'jql/jql.proto'
)
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\rjql/jql.proto\x12\x03jql\"\x13\n\x11ListTablesRequest\"7\n\tTableMeta\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x1c\n\x07\x63olumns\x18\x02 \x03(\x0b\x32\x0b.jql.Column\"4\n\x12ListTablesResponse\x12\x1e\n\x06tables\x18\x01 \x03(\x0b\x32\x0e.jql.TableMeta\"\x1b\n\nEqualMatch\x12\r\n\x05value\x18\x01 \x01(\t\"\x1e\n\rLessThanMatch\x12\r\n\x05value\x18\x01 \x01(\t\"!\n\x10GreaterThanMatch\x12\r\n\x05value\x18\x01 \x01(\t\"\x19\n\x07InMatch\x12\x0e\n\x06values\x18\x01 \x03(\t\"-\n\rContainsMatch\x12\r\n\x05\x65xact\x18\x01 \x01(\x08\x12\r\n\x05value\x18\x02 \x01(\t\"-\n\x0bPathToMatch\x12\r\n\x05value\x18\x01 \x01(\t\x12\x0f\n\x07reverse\x18\x02 \x01(\x08\"\xba\x02\n\x06\x46ilter\x12\x0f\n\x07negated\x18\x01 \x01(\x08\x12\x0e\n\x06\x63olumn\x18\x02 \x01(\t\x12&\n\x0b\x65qual_match\x18\x03 \x01(\x0b\x32\x0f.jql.EqualMatchH\x00\x12-\n\x0fless_than_match\x18\x04 \x01(\x0b\x32\x12.jql.LessThanMatchH\x00\x12\x34\n\x13greather_than_match\x18\x05 \x01(\x0b\x32\x15.jql.GreaterThanMatchH\x00\x12 \n\x08in_match\x18\x06 \x01(\x0b\x32\x0c.jql.InMatchH\x00\x12,\n\x0e\x63ontains_match\x18\x07 \x01(\x0b\x32\x12.jql.ContainsMatchH\x00\x12)\n\rpath_to_match\x18\x08 \x01(\x0b\x32\x10.jql.PathToMatchH\x00\x42\x07\n\x05match\"*\n\tCondition\x12\x1d\n\x08requires\x18\x01 \x03(\x0b\x32\x0b.jql.Filter\"\xa2\x01\n\x0fListRowsRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\"\n\nconditions\x18\x02 \x03(\x0b\x32\x0e.jql.Condition\x12\x10\n\x08order_by\x18\x03 \x01(\t\x12\x0b\n\x03\x64\x65\x63\x18\x04 \x01(\x08\x12\x0e\n\x06offset\x18\x05 \x01(\r\x12\r\n\x05limit\x18\x06 \x01(\r\x12\x1e\n\x08group_by\x18\x07 \x01(\x0b\x32\x0c.jql.GroupBy\"\x80\x01\n\x06\x43olumn\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x1c\n\x04type\x18\x02 \x01(\x0e\x32\x0e.jql.EntryType\x12\x12\n\nmax_length\x18\x03 \x01(\x05\x12\x0f\n\x07primary\x18\x04 \x01(\x08\x12\x15\n\rforeign_table\x18\x05 \x01(\t\x12\x0e\n\x06values\x18\x06 \x03(\t\"\x1a\n\x05\x45ntry\x12\x11\n\tformatted\x18\x01 \x01(\t\"\"\n\x03Row\x12\x1b\n\x07\x65ntries\x18\x01 \x03(\x0b\x32\n.jql.Entry\"\x95\x01\n\x10ListRowsResponse\x12\r\n\x05table\x18\x01 \x01(\t\x12\x1c\n\x07\x63olumns\x18\x02 \x03(\x0b\x32\x0b.jql.Column\x12\x16\n\x04rows\x18\x03 \x03(\x0b\x32\x08.jql.Row\x12\r\n\x05total\x18\x04 \x01(\r\x12\x0b\n\x03\x61ll\x18\x05 \x01(\r\x12 \n\tgroupings\x18\x06 \x03(\x0b\x32\r.jql.Grouping\"*\n\rGetRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\"T\n\x0eGetRowResponse\x12\r\n\x05table\x18\x01 \x01(\t\x12\x1c\n\x07\x63olumns\x18\x02 \x03(\x0b\x32\x0b.jql.Column\x12\x15\n\x03row\x18\x03 \x01(\x0b\x32\x08.jql.Row\"\xb7\x01\n\x0fWriteRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\x12\x30\n\x06\x66ields\x18\x03 \x03(\x0b\x32 .jql.WriteRowRequest.FieldsEntry\x12\x13\n\x0bupdate_only\x18\x04 \x01(\x08\x12\x13\n\x0binsert_only\x18\x05 \x01(\x08\x1a-\n\x0b\x46ieldsEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\t:\x02\x38\x01\"\x12\n\x10WriteRowResponse\"R\n\x15IncrementEntryRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\x12\x0e\n\x06\x63olumn\x18\x03 \x01(\t\x12\x0e\n\x06\x61mount\x18\x04 \x01(\x05\"\x18\n\x16IncrementEntryResponse\"-\n\x10\x44\x65leteRowRequest\x12\r\n\x05table\x18\x01 \x01(\t\x12\n\n\x02pk\x18\x02 \x01(\t\"\x13\n\x11\x44\x65leteRowResponse\"\x10\n\x0ePersistRequest\"\x11\n\x0fPersistResponse\"\x14\n\x12GetSnapshotRequest\"\'\n\x13GetSnapshotResponse\x12\x10\n\x08snapshot\x18\x01 \x01(\x0c\"\'\n\x13LoadSnapshotRequest\x12\x10\n\x08snapshot\x18\x01 \x01(\x0c\"\x16\n\x14LoadSnapshotResponse\"4\n\x11RequestedGrouping\x12\r\n\x05\x66ield\x18\x01 \x01(\t\x12\x10\n\x08selected\x18\x02 \x01(\t\"4\n\x07GroupBy\x12)\n\tgroupings\x18\x01 \x03(\x0b\x32\x16.jql.RequestedGrouping\"\x85\x01\n\x08Grouping\x12\r\n\x05\x66ield\x18\x01 \x01(\t\x12)\n\x06values\x18\x02 \x03(\x0b\x32\x19.jql.Grouping.ValuesEntry\x12\x10\n\x08selected\x18\x03 \x01(\t\x1a-\n\x0bValuesEntry\x12\x0b\n\x03key\x18\x01 \x01(\t\x12\r\n\x05value\x18\x02 \x01(\x03:\x02\x38\x01*\x80\x01\n\tEntryType\x12\n\n\x06STRING\x10\x00\x12\x07\n\x03INT\x10\x01\x12\x08\n\x04\x44\x41TE\x10\x02\x12\x08\n\x04\x45NUM\x10\x03\x12\x06\n\x02ID\x10\x04\x12\x08\n\x04TIME\x10\x05\x12\x0c\n\x08MONEYAMT\x10\x06\x12\x0b\n\x07\x46OREIGN\x10\x07\x12\x0c\n\x08\x46OREIGNS\x10\x08\x12\x0f\n\x0bPOLYFOREIGN\x10\t2\xad\x04\n\x03JQL\x12=\n\nListTables\x12\x16.jql.ListTablesRequest\x1a\x17.jql.ListTablesResponse\x12\x37\n\x08ListRows\x12\x14.jql.ListRowsRequest\x1a\x15.jql.ListRowsResponse\x12\x31\n\x06GetRow\x12\x12.jql.GetRowRequest\x1a\x13.jql.GetRowResponse\x12\x37\n\x08WriteRow\x12\x14.jql.WriteRowRequest\x1a\x15.jql.WriteRowResponse\x12:\n\tDeleteRow\x12\x15.jql.DeleteRowRequest\x1a\x16.jql.DeleteRowResponse\x12I\n\x0eIncrementEntry\x12\x1a.jql.IncrementEntryRequest\x1a\x1b.jql.IncrementEntryResponse\x12\x34\n\x07Persist\x12\x13.jql.PersistRequest\x1a\x14.jql.PersistResponse\x12@\n\x0bGetSnapshot\x12\x17.jql.GetSnapshotRequest\x1a\x18.jql.GetSnapshotResponse\x12\x43\n\x0cLoadSnapshot\x12\x18.jql.LoadSnapshotRequest\x1a\x19.jql.LoadSnapshotResponseB\x0bZ\tjql/jqlpbb\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'jql.jql_pb2', _globals)
if not _descriptor._USE_C_DESCRIPTORS:
  _globals['DESCRIPTOR']._loaded_options = None
  _globals['DESCRIPTOR']._serialized_options = b'Z\tjql/jqlpb'
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._loaded_options = None
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_options = b'8\001'
  _globals['_GROUPING_VALUESENTRY']._loaded_options = None
  _globals['_GROUPING_VALUESENTRY']._serialized_options = b'8\001'
  _globals['_ENTRYTYPE']._serialized_start=2168
  _globals['_ENTRYTYPE']._serialized_end=2296
  _globals['_LISTTABLESREQUEST']._serialized_start=22
  _globals['_LISTTABLESREQUEST']._serialized_end=41
  _globals['_TABLEMETA']._serialized_start=43
  _globals['_TABLEMETA']._serialized_end=98
  _globals['_LISTTABLESRESPONSE']._serialized_start=100
  _globals['_LISTTABLESRESPONSE']._serialized_end=152
  _globals['_EQUALMATCH']._serialized_start=154
  _globals['_EQUALMATCH']._serialized_end=181
  _globals['_LESSTHANMATCH']._serialized_start=183
  _globals['_LESSTHANMATCH']._serialized_end=213
  _globals['_GREATERTHANMATCH']._serialized_start=215
  _globals['_GREATERTHANMATCH']._serialized_end=248
  _globals['_INMATCH']._serialized_start=250
  _globals['_INMATCH']._serialized_end=275
  _globals['_CONTAINSMATCH']._serialized_start=277
  _globals['_CONTAINSMATCH']._serialized_end=322
  _globals['_PATHTOMATCH']._serialized_start=324
  _globals['_PATHTOMATCH']._serialized_end=369
  _globals['_FILTER']._serialized_start=372
  _globals['_FILTER']._serialized_end=686
  _globals['_CONDITION']._serialized_start=688
  _globals['_CONDITION']._serialized_end=730
  _globals['_LISTROWSREQUEST']._serialized_start=733
  _globals['_LISTROWSREQUEST']._serialized_end=895
  _globals['_COLUMN']._serialized_start=898
  _globals['_COLUMN']._serialized_end=1026
  _globals['_ENTRY']._serialized_start=1028
  _globals['_ENTRY']._serialized_end=1054
  _globals['_ROW']._serialized_start=1056
  _globals['_ROW']._serialized_end=1090
  _globals['_LISTROWSRESPONSE']._serialized_start=1093
  _globals['_LISTROWSRESPONSE']._serialized_end=1242
  _globals['_GETROWREQUEST']._serialized_start=1244
  _globals['_GETROWREQUEST']._serialized_end=1286
  _globals['_GETROWRESPONSE']._serialized_start=1288
  _globals['_GETROWRESPONSE']._serialized_end=1372
  _globals['_WRITEROWREQUEST']._serialized_start=1375
  _globals['_WRITEROWREQUEST']._serialized_end=1558
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_start=1513
  _globals['_WRITEROWREQUEST_FIELDSENTRY']._serialized_end=1558
  _globals['_WRITEROWRESPONSE']._serialized_start=1560
  _globals['_WRITEROWRESPONSE']._serialized_end=1578
  _globals['_INCREMENTENTRYREQUEST']._serialized_start=1580
  _globals['_INCREMENTENTRYREQUEST']._serialized_end=1662
  _globals['_INCREMENTENTRYRESPONSE']._serialized_start=1664
  _globals['_INCREMENTENTRYRESPONSE']._serialized_end=1688
  _globals['_DELETEROWREQUEST']._serialized_start=1690
  _globals['_DELETEROWREQUEST']._serialized_end=1735
  _globals['_DELETEROWRESPONSE']._serialized_start=1737
  _globals['_DELETEROWRESPONSE']._serialized_end=1756
  _globals['_PERSISTREQUEST']._serialized_start=1758
  _globals['_PERSISTREQUEST']._serialized_end=1774
  _globals['_PERSISTRESPONSE']._serialized_start=1776
  _globals['_PERSISTRESPONSE']._serialized_end=1793
  _globals['_GETSNAPSHOTREQUEST']._serialized_start=1795
  _globals['_GETSNAPSHOTREQUEST']._serialized_end=1815
  _globals['_GETSNAPSHOTRESPONSE']._serialized_start=1817
  _globals['_GETSNAPSHOTRESPONSE']._serialized_end=1856
  _globals['_LOADSNAPSHOTREQUEST']._serialized_start=1858
  _globals['_LOADSNAPSHOTREQUEST']._serialized_end=1897
  _globals['_LOADSNAPSHOTRESPONSE']._serialized_start=1899
  _globals['_LOADSNAPSHOTRESPONSE']._serialized_end=1921
  _globals['_REQUESTEDGROUPING']._serialized_start=1923
  _globals['_REQUESTEDGROUPING']._serialized_end=1975
  _globals['_GROUPBY']._serialized_start=1977
  _globals['_GROUPBY']._serialized_end=2029
  _globals['_GROUPING']._serialized_start=2032
  _globals['_GROUPING']._serialized_end=2165
  _globals['_GROUPING_VALUESENTRY']._serialized_start=2120
  _globals['_GROUPING_VALUESENTRY']._serialized_end=2165
  _globals['_JQL']._serialized_start=2299
  _globals['_JQL']._serialized_end=2856
# @@protoc_insertion_point(module_scope)
