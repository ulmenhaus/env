#! /usr/local/bin/python3

import json
import shutil

import click
import pydub
import requests

TRANSLATION_URL_TEMPLATE = "http://translate.google.com/translate_tts?tl=%(language_code)s&q=%(query)s"


class AudioGenerator(object):
    def __init__(self, vocab_dict, source_language_code, target_language_code):
        self.vocab_dict = vocab_dict
        self.source_language_code = source_language_code
        self.target_language_code = target_language_code

    def generate(self):
        for source_word, target_word in self.vocab_dict.items():
            src_filename = "%s_%s.mp3" % (self.source_language_code,
                                          source_word)
            self.get_audio(source_word, src_filename,
                           self.source_language_code)

            target_filename = "%s_%s.mp3" % (self.target_language_code,
                                             target_word)
            self.get_audio(target_word, target_filename,
                           self.target_language_code)

            src_sample = pydub.AudioSegment.from_mp3(src_filename)
            target_sample = pydub.AudioSegment.from_mp3(target_filename)
            post_src = pydub.AudioSegment.silent(duration=700)
            post_target = pydub.AudioSegment.silent(duration=1000)
            combined = src_sample + post_src + target_sample + post_target
            combined_filename = "%s_%s_%s.wav" % (self.source_language_code,
                                                  self.target_language_code,
                                                  source_word)
            tags = {
                'artist': 'Google Translate',
                'album': 'Translations',
            }
            combined.export(combined_filename, format="wav", tags=tags)

    def get_audio(self, word, outfile, language_code):
        from urllib.parse import quote, quote_plus
        url = TRANSLATION_URL_TEMPLATE % {
            'language_code': language_code,
            'query': word,
        }
        result = requests.get(
            url, stream=True, headers={'User-Agent': 'Mozilla/5.0'})
        with open(outfile, 'wb') as f:
            result.raw.decode_content = True
            shutil.copyfileobj(result.raw, f)


@click.argument('src')
@click.argument('target')
@click.argument('phrase')
@click.argument('translation')
def translate(src, target, phrase, translation):
    """
    SRC is the source language code
    TARGET is the target language code
    PHRASE is the phrase in the original language
    TRANSLATION is the phrase in the target language
    """
    AudioGenerator({phrase: translation}, src, target).generate()


def main():
    cli(obj={})


if __name__ == "__main__":
    main()
